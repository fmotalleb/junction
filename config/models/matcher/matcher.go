package matcher

type Matcher interface {
	Match(string) bool
	Validate() error
}
