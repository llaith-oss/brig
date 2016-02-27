package cmdline

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/disorganizer/brig/daemon"
	"github.com/disorganizer/brig/repo"
	repoconfig "github.com/disorganizer/brig/repo/config"
	"github.com/olebedev/config"
	"github.com/tsuibin/goxmpp2/xmpp"
	"github.com/tucnak/climax"
)

// guessRepoFolder tries to find the repository path
// by using a number of sources.
func guessRepoFolder() string {
	folder := repo.GuessFolder()
	if folder == "" {
		log.Errorf("This does not like a brig repository (missing .brig)")
		os.Exit(BadArgs)
	}

	return folder
}

func readPassword() (string, error) {
	repoFolder := guessRepoFolder()
	pwd, err := repo.PromptPasswordMaxTries(4, func(pwd string) bool {
		err := repo.CheckPassword(repoFolder, pwd)
		return err == nil
	})

	return pwd, err
}

type cmdHandlerWithClient func(ctx climax.Context, client *daemon.Client) int

func withDaemon(handler cmdHandlerWithClient, startNew bool) climax.CmdHandler {
	// If not, make sure we start a new one:
	return func(ctx climax.Context) int {
		port := guessPort()

		// Check if the daemon is running:
		client, err := daemon.Dial(port)
		if err == nil {
			return handler(ctx, client)
		}

		if !startNew {
			// Daemon was not running and we may not start a new one.
			log.Warning("Daemon not running.")
			return DaemonNotResponding
		}

		// Check if the password was supplied via a commandline flag.
		pwd, ok := ctx.Get("password")
		if !ok {
			// Prompt the user:
			var cmdPwd string

			cmdPwd, err = readPassword()
			if err != nil {
				log.Errorf("Could not read password: %v", pwd)
				return BadPassword
			}

			pwd = cmdPwd
		}

		// Start the dameon & pass the password:
		client, err = daemon.Reach(pwd, guessRepoFolder(), port)
		if err != nil {
			log.Errorf("Unable to start daemon: %v", err)
			return DaemonNotResponding
		}

		// Run the actual handler:
		return handler(ctx, client)
	}
}

type checkFunc func(ctx climax.Context) int

func withArgCheck(checker checkFunc, handler climax.CmdHandler) climax.CmdHandler {
	return func(ctx climax.Context) int {
		if checker(ctx) != Success {
			return BadArgs
		}

		return handler(ctx)
	}
}

func needAtLeast(min int) checkFunc {
	return func(ctx climax.Context) int {
		if len(ctx.Args) < min {
			log.Warningf("Need at least %d arguments.", min)
			return BadArgs
		}

		return Success
	}
}

func loadConfig() *config.Config {
	// We do not use guessRepoFolder() here. It might abort
	folder := repo.GuessFolder()
	cfg, err := repoconfig.LoadConfig(filepath.Join(folder, "config"))
	if err != nil {
		return repoconfig.CreateDefaultConfig()
	}

	return cfg
}

func guessPort() int {
	envPort := os.Getenv("BRIG_PORT")
	if envPort != "" {
		// Somebody tried to set BRIG_PORT.
		// Try to parse and spit errors if wrong.
		port, err := strconv.Atoi(envPort)
		if err != nil {
			log.Fatalf("Could not parse $BRIG_PORT: %v", err)
		}

		return port
	}

	// Trie the config elsewhise:
	config := loadConfig()
	port, err := config.Int("daemon.port")
	if err != nil {
		log.Fatalf("Cannot find out daemon port: %v", err)
	}

	return port
}

// checkJID runs sanity checks on a JID and returns a descriptive error.
func checkJID(jid xmpp.JID) error {
	if jid.Domain() == "" {
		return fmt.Errorf("Need a domain (user@domain.com/resource)")
	}

	if jid.Resource() == "" {
		return fmt.Errorf("Need a /resource (user@domain.com/resource)")
	}

	if jid.Node() == "" {
		return fmt.Errorf("Need a user (user@domain.com/resource)")
	}

	return nil
}
