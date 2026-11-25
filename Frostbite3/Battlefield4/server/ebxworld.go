package server

import (
	"sync"

	"bf4/ebx"
	"bf4/logger"
)

var (
	ebxOnce  sync.Once
	ebxWorld *ebx.World
)

func EBXWorld() *ebx.World {
	ebxOnce.Do(func() {
		w, err := ebx.LoadWorld(ebx.DefaultCatalogFile)
		if err != nil {
			logger.Warn("SERVER: EBX catalog: %v (using built-in default)", err)
			w = ebx.DefaultCatalog().Build()
		} else {
			logger.Info("SERVER: EBX catalog loaded: %s", w.DB)
		}
		ebxWorld = w
	})
	return ebxWorld
}
