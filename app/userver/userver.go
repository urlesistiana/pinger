package userver

import (
	"flag"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"net"
	"os"
	"os/signal"
	"pinger/app"
	"pinger/pkg/mlog"
	"pinger/pkg/udp_pinger"
	"syscall"
)

func init() {
	app.AddSubCmd(NewCmd())
}

func NewCmd() *cobra.Command {
	a := new(args)
	cmd := &cobra.Command{
		Use:   "userver [-a server_auth] [-l listen_addr:port]",
		Args:  cobra.NoArgs,
		Short: "Start a UDP ping server.",
		Run: func(cmd *cobra.Command, args []string) {
			run(a)
		},
		DisableFlagsInUseLine: true,
	}
	cmd.Flags().StringVarP(&a.listenAddr, "listen", "l", ":48330", "Listen address")
	cmd.Flags().StringVarP(&a.auth, "auth", "a", "", "Auth password")
	return cmd
}

type args struct {
	listenAddr string
	auth       string
}

var logger = mlog.L()

func run(a *args) {
	flag.Parse()

	c, err := net.ListenPacket("udp", a.listenAddr)
	if err != nil {
		logger.Fatal("failed to bind on socket", zap.Error(err))
	}
	defer c.Close()
	logger.Info("pinger pinger-server is started", zap.Stringer("addr", c.LocalAddr()))

	uc := c.(*net.UDPConn)
	go func() {
		err := udp_pinger.Server(uc, a.auth)
		logger.Fatal("pinger-server exited", zap.Error(err))
	}()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	sig := <-sc
	logger.Info("pinger pinger-server is exiting", zap.Stringer("signal", sig))
}
