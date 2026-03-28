package version

var (
	Version   = "dev"
	BuildTime = ""
	GoVersion = ""
	GitHash   = ""
)

func Info() map[string]string {
	return map[string]string{
		"Version":   Version,
		"BuildTime": BuildTime,
		"GoVersion": GoVersion,
		"GitHash":   GitHash,
	}
}
