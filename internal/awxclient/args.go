package awxclient

import "strconv"

// buildArgs renders the read-only `awxctl job info bmn` invocation. Region is
// auto-discovered by awxctl from BMN metadata, so -r is never passed. -o json
// is always appended so the output decodes into displayableJob. The verb is
// fixed to the read-only `job info bmn` path — see the §0.25 envelope note in
// the package doc.
func buildArgs(bmns []string, opts Options) []string {
	args := make([]string, 0, len(bmns)+8)
	args = append(args, "job", "info", "bmn")
	args = append(args, bmns...)
	args = append(args, "-l", opts.LimitType)
	if opts.PerTarget > 0 {
		args = append(args, "-n", strconv.Itoa(opts.PerTarget))
	}
	if opts.Template != "" {
		args = append(args, "-t", opts.Template)
	}
	args = append(args, "-o", "json")
	return args
}
