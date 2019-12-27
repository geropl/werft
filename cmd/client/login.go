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

	v1 "github.com/32leaves/werft/pkg/api/v1"
	"github.com/spf13/cobra"
)

// loginCmd represents the job command
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to werft",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := dial()
		defer conn.Close()
		client := v1.NewWerftServiceClient(conn)

		ctx := context.Background()
		evts, err := client.Login(ctx, &v1.LoginRequest{})
		if err != nil {
			return err
		}

		var token string
		for {
			msg, err := evts.Recv()
			if err != nil {
				return err
			}

			if url := msg.GetUrl(); url != "" {
				fmt.Printf("Please visit this URL to complete the login:\n\t%s\n\n", url)
				continue
			}

			token = msg.GetToken()
			if token != "" {
				break
			}
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		fn := filepath.Join(home, ".werft", "token")
		err = os.MkdirAll(filepath.Dir(fn), 0755)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(fn, []byte(token), 0600)
		if err != nil {
			return err
		}

		fmt.Println("success")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
