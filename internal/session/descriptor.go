package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const descriptorLimit = 64 * 1024
const healthLimit = 4 * 1024

// Descriptor separates public instance identity from the private API capability.
type Descriptor struct {
	Version          int    `json:"version"`
	ProjectID        string `json:"project_id"`
	PID              int    `json:"pid"`
	InstanceID       string `json:"instance_id"`
	Nonce            string `json:"nonce"`
	Role             string `json:"role"`
	UIURL            string `json:"ui_url"`
	ReceiverEndpoint string `json:"receiver_endpoint,omitempty"`
}

func NewDescriptor(projectID, role string) (Descriptor, error) {
	d := Descriptor{Version: 1, ProjectID: projectID, PID: os.Getpid(), Role: role}
	if !validHex(projectID, 16) || (role != "run" && role != "history") {
		return Descriptor{}, errors.New("invalid session project or role")
	}
	identity, nonce := make([]byte, 16), make([]byte, 32)
	if _, err := rand.Read(identity); err != nil {
		return Descriptor{}, errors.New("generate session identity failed")
	}
	if _, err := rand.Read(nonce); err != nil {
		return Descriptor{}, errors.New("generate session authorization failed")
	}
	d.InstanceID, d.Nonce = hex.EncodeToString(identity), hex.EncodeToString(nonce)
	return d, nil
}

// Publish atomically replaces the descriptor. The caller must hold the project ownership lock.
func Publish(path string, d Descriptor) error {
	if err := d.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return errors.New("encode session descriptor failed")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("session descriptor must be a regular file")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return fmt.Errorf("create session descriptor: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write session descriptor: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync session descriptor: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session descriptor: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("publish session descriptor: %w", err)
	}
	return nil
}

func Read(path string) (Descriptor, error) {
	var d Descriptor
	info, err := os.Lstat(path)
	if err != nil {
		return d, fmt.Errorf("read session descriptor: %w", err)
	}
	if !info.Mode().IsRegular() {
		return d, errors.New("session descriptor must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return d, errors.New("session descriptor permissions must be private")
	}
	file, err := os.Open(path)
	if err != nil {
		return d, fmt.Errorf("open session descriptor: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, descriptorLimit+1))
	if err != nil {
		return d, fmt.Errorf("read session descriptor: %w", err)
	}
	if len(data) > descriptorLimit {
		return d, errors.New("session descriptor exceeds 64 KiB")
	}
	if err := decodeJSON(data, &d); err != nil {
		return Descriptor{}, errors.New("invalid session descriptor JSON")
	}
	if err := d.validate(); err != nil {
		return Descriptor{}, err
	}
	return d, nil
}

// Probe validates public identity only; it never sends the private API nonce to an unverified port.
func Probe(ctx context.Context, d Descriptor) error {
	if err := d.validate(); err != nil {
		return err
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(d.UIURL, "/")+"/healthz", nil)
	if err != nil {
		return errors.New("invalid session health request")
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("session health request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("session health endpoint did not return success")
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("session health endpoint returned an unsupported content type")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, healthLimit+1))
	if err != nil {
		return fmt.Errorf("read session health response: %w", err)
	}
	if len(data) > healthLimit {
		return errors.New("session health response exceeds 4 KiB")
	}
	var health struct {
		Version    int    `json:"version"`
		ProjectID  string `json:"project_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := decodeJSON(data, &health); err != nil {
		return errors.New("invalid session health response")
	}
	if health.Version != d.Version || health.ProjectID != d.ProjectID || health.InstanceID != d.InstanceID {
		return errors.New("session endpoint belongs to a different instance")
	}
	return nil
}

// Remove deletes only the named instance. The caller must hold the project ownership lock.
func Remove(path, instanceID string) error {
	if !validHex(instanceID, 16) {
		return errors.New("invalid session instance identity")
	}
	d, err := Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if d.InstanceID != instanceID {
		return errors.New("session descriptor belongs to another instance")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove session descriptor: %w", err)
	}
	return nil
}

func (d Descriptor) validate() error {
	if d.Version != 1 {
		return errors.New("unsupported session descriptor version")
	}
	if d.PID <= 0 || !validHex(d.ProjectID, 16) || !validHex(d.InstanceID, 16) || !validHex(d.Nonce, 32) {
		return errors.New("invalid session identity")
	}
	if d.Role != "run" && d.Role != "history" {
		return errors.New("invalid session role")
	}
	if !validLocalURL(d.UIURL, false) {
		return errors.New("invalid session UI URL; require HTTP on literal 127.0.0.1 with a port")
	}
	if d.Role == "run" && !validLocalURL(d.ReceiverEndpoint, true) {
		return errors.New("invalid session receiver endpoint")
	}
	if d.Role == "history" && d.ReceiverEndpoint != "" {
		return errors.New("history session must not have a receiver endpoint")
	}
	return nil
}

func validHex(value string, size int) bool {
	if len(value) != size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size
}
func validLocalURL(value string, receiver bool) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(value, "#") || u.RawPath != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Host != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
		return false
	}
	if receiver {
		return u.Path == "/v1/traces"
	}
	return u.Path == "" || u.Path == "/"
}

func decodeJSON(data []byte, target any) error {
	// Typed decoding matches keys without case sensitivity and otherwise accepts the last value.
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("expected JSON object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		name = strings.ToLower(name)
		if !ok || seen[name] {
			return errors.New("duplicate or invalid JSON field")
		}
		seen[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
