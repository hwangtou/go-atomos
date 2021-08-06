package go_atomos

// CHECKED!

import "errors"

// Config Error

var (
	ErrConfigIsNil          = errors.New("config not found")
	ErrConfigNodeInvalid    = errors.New("config node name is invalid")
	ErrConfigLogPathInvalid = errors.New("config log path is invalid")
)

func (x *Config) check(c *CosmosSelf) error {
	if x == nil {
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
