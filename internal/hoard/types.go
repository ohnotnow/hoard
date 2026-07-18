package hoard

// Version is stamped at release time via -ldflags.
var Version = "dev"

type CLIError struct {
	Code     string
	Message  string
	ExitCode int
}

func (e *CLIError) Error() string { return e.Message }

func usageError(msg string) *CLIError {
	return &CLIError{Code: "usage", Message: msg, ExitCode: 64}
}

func validationError(msg string) *CLIError {
	return &CLIError{Code: "validation", Message: msg, ExitCode: 65}
}

func notFoundError(msg string) *CLIError {
	return &CLIError{Code: "not_found", Message: msg, ExitCode: 66}
}

func internalError(msg string) *CLIError {
	return &CLIError{Code: "internal", Message: msg, ExitCode: 1}
}

// noMatch signals an empty query result: not an error, but exit 1 so shell
// scripts and agents can branch on it.
func noMatch() *CLIError {
	return &CLIError{Code: "no_match", ExitCode: 1}
}

type TitleMatch struct {
	Title   string   `json:"title"`
	Key     string   `json:"key"`
	Videos  int      `json:"videos"`
	Audio   int      `json:"audio"`
	Sources []string `json:"sources"`
	Samples []string `json:"samples"`
}

type TitleRow struct {
	Items   int    `json:"items"`
	Title   string `json:"title"`
	Key     string `json:"key"`
	Videos  int    `json:"videos"`
	Audio   int    `json:"audio"`
	Sources int    `json:"sources"`
}

type SourceRow struct {
	Label      string `json:"label"`
	Files      int    `json:"files"`
	Videos     int    `json:"videos"`
	Audio      int    `json:"audio"`
	IngestedAt string `json:"ingested_at"`
}

type AliasRow struct {
	Alias string `json:"alias"`
	Title string `json:"title"`
}

type ExtCount struct {
	Ext   string `json:"ext"`
	Count int    `json:"count"`
}

type IngestResult struct {
	Source string `json:"source"`
	Files  int    `json:"files"`
	Videos int    `json:"videos"`
	Audio  int    `json:"audio"`
	Other  int    `json:"other"`
	Titles int    `json:"titles"`
}

type StatsResult struct {
	Files      int         `json:"files"`
	Videos     int         `json:"videos"`
	Audio      int         `json:"audio"`
	Other      int         `json:"other"`
	Titles     int         `json:"titles"`
	Aliases    int         `json:"aliases"`
	Sources    []SourceRow `json:"sources"`
	Extensions []ExtCount  `json:"extensions"`
}
