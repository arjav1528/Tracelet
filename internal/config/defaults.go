package config

func defaults() Config {
	var c Config
	c.Version = 1
	c.Collector.Bind, c.UI.Bind = "127.0.0.1", "127.0.0.1"
	c.Collector.Protocol = "otlp-http-protobuf"
	c.UI.OpenBrowser = true
	c.Storage.RetentionDays, c.Storage.MaxSizeMB = 7, 1024
	c.ShutdownGraceSeconds = 5
	c.Source.WorkspaceRoot = "."
	c.Limits = Limits{
		MaxWireBytes: 4 << 20, MaxDecodedBytes: 16 << 20, MaxSpansPerBatch: 5000,
		MaxConcurrentRequests: 4, MaxAttributes: 128, MaxKeyBytes: 256, MaxValueBytes: 4096,
		MaxDepth: 8, MaxEvents: 128, MaxLinks: 128, MaxPendingBatches: 8, MaxPendingBytes: 32 << 20,
		ReadHeaderTimeoutSeconds: 5, ReadTimeoutSeconds: 15, WriteTimeoutSeconds: 15, IdleTimeoutSeconds: 60,
	}
	return c
}
