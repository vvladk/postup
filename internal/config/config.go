package config

import "embed"

type Config struct {
	Port  string
	GetIP func() string
	FS    embed.FS
}
