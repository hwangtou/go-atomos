package go_atomos

func (x *Config) check(c *CosmosSelf) error {
	if x == nil {
		// TODO
		return ErrConfigIsNil
	}
	if x.Node == "" {
		return ErrConfigNodeInvalid
	}
	if x.LogPath == "" {
		return ErrConfigLogPathInvalid
	}
	// TODO: Try open log file
	return nil
}
