package backup

// SetMemberLimitForTest lowers the member size so a small database splits; call the
// returned func to restore it.
func SetMemberLimitForTest(n int64) func() {
	old := memberLimit
	memberLimit = n
	return func() { memberLimit = old }
}
