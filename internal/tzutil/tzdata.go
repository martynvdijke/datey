package tzutil

// Embed the IANA timezone database so time.LoadLocation works identically in
// minimal container images that ship no /usr/share/zoneinfo. This is the
// standard-library bundle (import only, no external dependency) and costs
// roughly 450 KiB in the binary.
import _ "time/tzdata"
