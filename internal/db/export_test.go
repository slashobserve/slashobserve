package db

func SetSpecUpgradeBatch(n int) (restore func()) {
	previous := specUpgradeBatch
	specUpgradeBatch = n
	return func() { specUpgradeBatch = previous }
}
