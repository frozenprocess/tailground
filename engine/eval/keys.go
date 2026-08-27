package eval

import "fmt"

// PairKey is the "src->dst" key of Result.Matrices, Result.Grants and
// PostureReport.Denials. Each end is a device ID or a matrix column
// ("route:…", "svc:…", "ext:…").
func PairKey(src, dst any) string { return fmt.Sprintf("%v->%v", src, dst) }

// SSHKey is the "user@srcID->dstID" key of Result.SSH.
func SSHKey(user string, src, dst any) string { return user + "@" + PairKey(src, dst) }
