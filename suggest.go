package frictionax

// suggester provides command/flag suggestions for CLI errors.
type suggester interface {
	Suggest(input string, ctx suggestContext) *Suggestion
}
