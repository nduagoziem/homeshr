package cache

// PropertyLockKey is the Redis key for the distributed lock that serializes
// booking operations against a single property, preventing two concurrent
// requests from both passing the overlap check and double-booking the dates.
func PropertyLockKey(propertyID string) string {
	return "lock:property:" + propertyID
}
