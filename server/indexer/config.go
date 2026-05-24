package indexer

// Config configures the workspace indexer.
type Config struct {
	Associations []string
	Exclude      []string
	MaxSize      int64
	StubsPath    string
	Stubs        []string
	PHPVersion   string
}

// WorkspaceFolder is a root folder provided by the LSP client.
type WorkspaceFolder struct {
	URI  string
	Name string
}
