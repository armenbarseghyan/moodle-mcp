package extract

// SetZipBudget lowers the archive budget for a test and returns a restore func.
func SetZipBudget(n int64) func() {
	old := maxZipTotal
	maxZipTotal = n
	return func() { maxZipTotal = old }
}
