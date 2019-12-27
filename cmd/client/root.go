package cmd

// Copyright © 2019 Christian Weichel

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	verbose bool
	host    string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "werft",
	Short: "werft is a very simple GitHub triggered and Kubernetes powered CI system",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			log.SetLevel(log.DebugLevel)
			log.Debug("verbose logging enabled")
		}
	},

	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	werftHost := os.Getenv("WERFT_HOST")
	if werftHost == "" {
		werftHost = "localhost:7777"
	}

	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "en/disable verbose logging")
	rootCmd.PersistentFlags().StringVar(&host, "host", werftHost, "werft host to talk to (defaults to WERFT_HOST env var)")
}

func dial() *grpc.ClientConn {
	conn, err := grpc.Dial(host, grpc.WithInsecure())
	if err != nil {
		log.WithError(err).Fatal("cannot connect to werft server")
	}

	return conn
}

func withToken(ctx context.Context) context.Context {
	home, err := os.UserHomeDir()
	if err != nil {
		return ctx
	}

	fn := filepath.Join(home, ".werft", "token")
	tkn, err := ioutil.ReadFile(fn)
	if err != nil {
		return ctx
	}

	md := metadata.New(map[string]string{"authorization": string(tkn)})
	return metadata.NewOutgoingContext(ctx, md)
}
