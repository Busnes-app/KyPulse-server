package monitor

// Flush waits until every delivery queued before it has been sent. The sender must be running.
func Flush(s *Service) {
	done := make(chan struct{})
	s.queue <- delivery{done: done}
	<-done
}
