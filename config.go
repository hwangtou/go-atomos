package go_atomos

func (x *Config) Check() *ErrorInfo {
	if x == nil {
		return NewError(ErrCosmosConfigInvalid, "No configuration")
	}
	if x.Node == "" {
		return NewError(ErrCosmosConfigNodeNameInvalid, "Node name is empty")
	}
	if x.LogPath == "" {
		return NewError(ErrCosmosConfigLogPathInvalid, "Log path is empty")
	}
	// TODO: Try open log file
	return nil
}
