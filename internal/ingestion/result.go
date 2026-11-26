package ingestion

// Result describes committed ingestion; retries can be duplicates without adding rows.
type Result struct {
	Accepted   int
	Duplicates int
	Conflicts  int
}
