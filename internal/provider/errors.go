package provider

type SshConnectError struct {
	inner error
}

func (e *SshConnectError) Error() string {
	return e.inner.Error()
}

type StderrError struct {
	stderr string
}

func (e *StderrError) Error() string {
	return e.stderr
}

type DatasetError struct {
	errmsg string
}

func (e *DatasetError) Error() string {
	return e.errmsg
}

type PoolError struct {
	errmsg string
}

func (e *PoolError) Error() string {
	return e.errmsg
}
