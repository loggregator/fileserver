package handlers

import (
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	steno "github.com/cloudfoundry/gosteno"
)

func New(c *config.Config, logger *steno.Logger) router.Handlers {
	return router.Handlers{
		router.FS_STATIC:         static.New(c.StaticDirectory),
		router.FS_UPLOAD_DROPLET: upload_droplet.New(c.CCAddress, c.CCUsername, c.CCPassword, c.CCJobPollingInterval, c.SkipCertVerify, logger),
	}
}
