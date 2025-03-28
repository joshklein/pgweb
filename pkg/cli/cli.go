package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"

	"github.com/sosedoff/pgweb/pkg/api"
	"github.com/sosedoff/pgweb/pkg/bookmarks"
	"github.com/sosedoff/pgweb/pkg/client"
	"github.com/sosedoff/pgweb/pkg/command"
	"github.com/sosedoff/pgweb/pkg/connection"
	"github.com/sosedoff/pgweb/pkg/shared"
	"github.com/sosedoff/pgweb/pkg/util"
)

var (
	logger  *logrus.Logger
	options command.Options

	readonlyWarning = `
------------------------------------------------------
SECURITY WARNING: You are running pgweb in read-only mode.
This mode is designed for environments where users could potentially delete / change data.
For proper read-only access please follow postgresql role management documentation.
------------------------------------------------------`

	regexErrConnectionRefused = regexp.MustCompile(`(connection|actively) refused`)
	regexErrAuthFailed        = regexp.MustCompile(`authentication failed`)
)

func init() {
	logger = logrus.New()
}

func exitWithMessage(message string) {
	fmt.Println("Error:", message)
	os.Exit(1)
}

func initClientUsingBookmark(bookmarkPath, bookmarkName string) (*client.Client, error) {
	bookmark, err := bookmarks.GetBookmark(bookmarkPath, bookmarkName)
	if err != nil {
		return nil, err
	}

	opt := bookmark.ConvertToOptions()
	var connStr string

	if opt.URL != "" { // if the bookmark has url set, use it
		connStr = opt.URL
	} else {
		connStr, err = connection.BuildStringFromOptions(opt)
		if err != nil {
			return nil, fmt.Errorf("error building connection string: %v", err)
		}
	}

	var ssh *shared.SSHInfo
	if !bookmark.SSHInfoIsEmpty() {
		ssh = bookmark.SSH
	}

	return client.NewFromUrl(connStr, ssh)
}

func initClient() {
	if connection.IsBlank(command.Opts) && options.Bookmark == "" {
		return
	}

	var cl *client.Client
	var err error

	if options.Bookmark != "" {
		cl, err = initClientUsingBookmark(bookmarks.Path(options.BookmarksDir), options.Bookmark)
	} else {
		cl, err = client.New()
	}
	if err != nil {
		exitWithMessage(err.Error())
	}

	if command.Opts.Debug {
		fmt.Println("Server connection string:", cl.ConnectionString)
	}

	fmt.Println("Connecting to server...")
	if err := cl.Test(); err != nil {
		msg := err.Error()

		// Check if we're trying to connect to the default database.
		if command.Opts.DbName == "" && command.Opts.URL == "" {
			// If database does not exist, allow user to connect from the UI.
			if strings.Contains(msg, "database") && strings.Contains(msg, "does not exist") {
				fmt.Println("Error:", msg)
				return
			}

			// Do not bail if local server is not running.
			if regexErrConnectionRefused.MatchString(msg) {
				fmt.Println("Error:", msg)
				return
			}

			// Do not bail if local auth is invalid
			if regexErrAuthFailed.MatchString(msg) {
				fmt.Println("Error:", msg)
				return
			}
		}

		exitWithMessage(msg)
	}

	if !command.Opts.Sessions {
		fmt.Printf("Connected to %s\n", cl.ServerVersionInfo())
	}

	fmt.Println("Checking database objects...")
	_, err = cl.Objects()
	if err != nil {
		exitWithMessage(err.Error())
	}

	api.DbClient = cl
}

func initOptions() {
	opts, err := command.ParseOptions(os.Args)
	if err != nil {
		switch errVal := err.(type) {
		case *flags.Error:
			if errVal.Type == flags.ErrHelp {
				fmt.Println("Available environment variables:")
				fmt.Println(command.AvailableEnvVars())
			}
			// no need to print error, flags package already does that
		default:
			fmt.Println(err.Error())
		}
		os.Exit(1)
	}
	command.Opts = opts
	options = opts

	if options.Version {
		printVersion()
		os.Exit(0)
	}

	if err := configureLogger(opts); err != nil {
		exitWithMessage(err.Error())
		return
	}

	if options.ReadOnly {
		fmt.Println(readonlyWarning)
	}

	if options.BinaryCodec != "" {
		if err := client.SetBinaryCodec(options.BinaryCodec); err != nil {
			exitWithMessage(err.Error())
		}
	}

	printVersion()
}

func configureLogger(opts command.Options) error {
	if options.Debug {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		lvl, err := logrus.ParseLevel(opts.LogLevel)
		if err != nil {
			return err
		}
		logger.SetLevel(lvl)
	}

	switch options.LogFormat {
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{})
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		return fmt.Errorf("invalid logger format: %v", options.LogFormat)
	}

	return nil
}

func printVersion() {
	fmt.Println(command.VersionString())
}

func startServer() {
	router := gin.New()
	router.Use(api.RequestLogger(logger))
	router.Use(gin.Recovery())

	// Enable HTTP basic authentication only if both user and password are set
	if options.AuthUser != "" && options.AuthPass != "" {
		auth := map[string]string{options.AuthUser: options.AuthPass}
		router.Use(gin.BasicAuth(auth))
	}

	api.SetLogger(logger)
	api.SetupRoutes(router)

	fmt.Println("Starting server...")
	go func() {
		err := router.Run(fmt.Sprintf("%v:%v", options.HTTPHost, options.HTTPPort))
		if err != nil {
			fmt.Println("Cant start server:", err)
			if strings.Contains(err.Error(), "address already in use") {
				openPage()
			}
			os.Exit(1)
		}
	}()
}

func handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}

func openPage() {
	url := fmt.Sprintf("http://%v:%v/%s", options.HTTPHost, options.HTTPPort, options.Prefix)
	fmt.Println("To view database open", url, "in browser")

	if options.SkipOpen {
		return
	}

	_, err := exec.Command("which", "open").Output()
	if err != nil {
		return
	}

	_, err = exec.Command("open", url).Output()
	if err != nil {
		fmt.Println("Unable to auto-open pgweb URL:", err)
	}
}

func Run() {
	initOptions()
	initClient()

	if api.DbClient != nil {
		defer api.DbClient.Close()
	}

	if !options.Debug {
		gin.SetMode("release")
	}

	// Print memory usage every 30 seconds with debug flag
	if options.Debug {
		util.StartProfiler()
	}

	// Start session cleanup worker
	if options.Sessions {
		api.DbSessions = api.NewSessionManager(logger)

		if !command.Opts.DisableConnectionIdleTimeout {
			api.DbSessions.SetIdleTimeout(time.Minute * time.Duration(command.Opts.ConnectionIdleTimeout))
			go api.DbSessions.RunPeriodicCleanup()
		}
	}

	startServer()
	openPage()
	handleSignals()
}
