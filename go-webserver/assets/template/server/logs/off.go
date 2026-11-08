// whole-file feature:!logs
// Package logs is what is left of the activity log in a project without it: the
// names the rest of the server uses to say something happened, so that every
// route can go on saying it, and nothing written down. Redact and Truncate are
// real: see redact.go.
package logs

// Kinds. Nothing keeps them; they are what the routes pass.
const (
	KindAuth  = "auth"
	KindAdmin = "admin"
)

const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Actor is who an entry would belong to.
type Actor struct{}

func Anonymous() ( actor *Actor ) {
	return
}

// Logger writes nothing.
type Logger struct{}

func ( logger *Logger ) System( level string , message string , detail map[string]any ) {}
