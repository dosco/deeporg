package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"github.com/spf13/cobra"
)

// resetPasswordCmd is the way back in for whoever runs the server. An admin
// who has lost their password cannot ask another admin when there is none,
// but they can run a command on the machine the server runs on.
func resetPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <email>",
		Short: "Make a link for choosing a new password",
		Long: `Make a link with which the person with this email chooses a new password.

Run it on the machine DeepORG runs on. The server does not have to be stopped.
The link works once, for 24 hours. Choosing the new password signs that account
out everywhere else, including any agent it connected.

An admin can make the same link for anyone from Admin, then Setup, in the
console. This command is for an admin who cannot sign in.

Example:
  deeporg reset-password ada@example.com`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			sc, err := existingStartConfig(cmd)
			if err != nil {
				log.Fatalf("%s", err)
			}
			link, expires, err := resetPasswordLink(sc, args[0])
			if err != nil {
				log.Fatalf("%s", err)
			}
			fmt.Printf("Open this link to choose a new password for %s:\n\n  %s\n\n", strings.ToLower(strings.TrimSpace(args[0])), link)
			fmt.Printf("It works once, until %s. If you open it on another computer, replace the\nfirst part with your server's address.\n", expires.Local().Format("15:04 on 2 January"))
		},
	}
}

// resetPasswordLink writes a password link for this email into the store and
// returns the address to open, as the server itself would be reached.
func resetPasswordLink(sc *startConfig, email string) (string, time.Time, error) {
	token, expires, err := serv.CreatePasswordReset(context.Background(), sc.Store, email)
	if err != nil {
		if serv.IsNoSuchAccount(err) {
			return "", time.Time{}, fmt.Errorf("nobody has an account with the email %s on this server", strings.TrimSpace(email))
		}
		return "", time.Time{}, err
	}
	return "http://" + displayListen(sc.Listen) + serv.ResetLinkPath(token), expires, nil
}
