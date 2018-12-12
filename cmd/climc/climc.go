package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/c-bata/go-prompt"
	"yunion.io/x/log"
	"yunion.io/x/pkg/util/version"
	"yunion.io/x/structarg"

	"yunion.io/x/onecloud/cmd/climc/promputils"
	"yunion.io/x/onecloud/cmd/climc/shell"
	_ "yunion.io/x/onecloud/cmd/climc/shell/k8s"
	"yunion.io/x/onecloud/pkg/mcclient"
)

type BaseOptions struct {
	Help       bool   `help:"Show help" short-token:"h"`
	Debug      bool   `help:"Show debug information"`
	Version    bool   `help:"Show version"`
	Timeout    int    `default:"600" help:"Number of seconds to wait for a response"`
	Insecure   bool   `default:"False" help:"Allow skip server cert verification if URL is https" short-token:"k"`
	OsUsername string `default:"$OS_USERNAME" help:"Username, defaults to env[OS_USERNAME]"`
	OsPassword string `default:"$OS_PASSWORD" help:"Password, defaults to env[OS_PASSWORD]"`
	// OsProjectId string `default:"$OS_PROJECT_ID" help:"Proejct ID, defaults to env[OS_PROJECT_ID]"`
	OsProjectName  string `default:"$OS_PROJECT_NAME" help:"Project name, defaults to env[OS_PROJECT_NAME]"`
	OsDomainName   string `default:"$OS_DOMAIN_NAME" help:"Domain name, defaults to env[OS_DOMAIN_NAME]"`
	OsAuthURL      string `default:"$OS_AUTH_URL" help:"Defaults to env[OS_AUTH_URL]"`
	OsRegionName   string `default:"$OS_REGION_NAME" help:"Defaults to env[OS_REGION_NAME]"`
	OsZoneName     string `default:"$OS_ZONE_NAME" help:"Defaults to env[OS_ZONE_NAME]"`
	OsEndpointType string `default:"$OS_ENDPOINT_TYPE|internalURL" help:"Defaults to env[OS_ENDPOINT_TYPE] or internalURL" choices:"publicURL|internalURL|adminURL"`
	ApiVersion     string `default:"$API_VERSION|v1" help:"apiVersion, default to v1"`
	SUBCOMMAND     string `help:"climc subcommand" subcommand:"true"`
}

func getSubcommandsParser() (*structarg.ArgumentParser, error) {
	parse, e := structarg.NewArgumentParser(&BaseOptions{},
		"climc",
		`Command-line interface to the API server.`,
		`See "climc help COMMAND" for help on a specific command.`)
	if e != nil {
		return nil, e
	}
	subcmd := parse.GetSubcommand()
	if subcmd == nil {
		return nil, fmt.Errorf("No subcommand argument")
	}
	type HelpOptions struct {
		SUBCOMMAND string `help:"Sub-command name"`
	}
	shell.R(&HelpOptions{}, "help", "Show help information of any subcommand", func(suboptions *HelpOptions) error {
		helpstr, e := subcmd.SubHelpString(suboptions.SUBCOMMAND)
		if e != nil {
			return e
		} else {
			fmt.Print(helpstr)
			return nil
		}
	})
	for _, v := range shell.CommandTable {
		_par, e := subcmd.AddSubParser(v.Options, v.Command, v.Desc, v.Callback)

		if e != nil {
			return nil, e
		}
		promputils.AppendCommand(v.Command, v.Desc)
		cmd := v.Command

		for _, v := range _par.GetOptArgs() {
			_name := strings.Replace(v.String(), "]", "", -1)
			_name = strings.Replace(_name, "[", "", -1)
			promputils.AppendOpt(cmd, _name, v.HelpString(""))

		}
		for _, v := range _par.GetPosArgs() {
			_name := strings.Replace(v.String(), "<", "", -1)
			_name = strings.Replace(_name, ">", "", -1)
			promputils.AppendPos(cmd, _name, v.HelpString(""))
		}
	}
	return parse, nil
}

func showErrorAndExit(e error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", e)
	os.Exit(1)
}

func newClientSession(options *BaseOptions) (*mcclient.ClientSession, error) {
	if len(options.OsAuthURL) == 0 {
		return nil, fmt.Errorf("Missing OS_AUTH_URL")
	}
	if len(options.OsUsername) == 0 {
		return nil, fmt.Errorf("Missing OS_USERNAME")
	}
	if len(options.OsPassword) == 0 {
		return nil, fmt.Errorf("Missing OS_PASSWORD")
	}
	if len(options.OsRegionName) == 0 {
		return nil, fmt.Errorf("Missing OS_REGION_NAME")
	}
	// if len(options.OsProjectId) == 0 && len(options.OsProjectName) == 0 {
	//    showErrorAndExit(fmt.Errorf("Missing OS_PROEJCT_ID or OS_PROJECT_NAME"))
	if len(options.OsProjectName) == 0 {
		return nil, fmt.Errorf("Missing OS_PROJECT_NAME")
	}

	logLevel := "info"
	if options.Debug {
		logLevel = "debug"
	}
	log.SetLogLevelByString(log.Logger(), logLevel)

	client := mcclient.NewClient(options.OsAuthURL,
		options.Timeout,
		options.Debug,
		options.Insecure)
	token, err := client.Authenticate(options.OsUsername,
		options.OsPassword,
		options.OsDomainName,
		options.OsProjectName)
	if err != nil {
		return nil, err
	}

	session := client.NewSession(options.OsRegionName,
		options.OsZoneName,
		options.OsEndpointType,
		token,
		options.ApiVersion)
	return session, nil
}

func enterInteractiveMode(
	parser *structarg.ArgumentParser,
	sessionFactory func() *mcclient.ClientSession,
) {
	promputils.InitEnv(parser, sessionFactory())
	defer fmt.Println("Bye!")
	p := prompt.New(
		promputils.Executor,
		promputils.Completer,
		prompt.OptionPrefix("climc> "),
		prompt.OptionTitle("Climc, a Command Line Interface to Manage Clouds"),
		prompt.OptionMaxSuggestion(16),
	)
	p.Run()
}

func executeSubcommand(
	subcmd *structarg.SubcommandArgument,
	subparser *structarg.ArgumentParser,
	options *BaseOptions,
	sessionFactory func() *mcclient.ClientSession,
) {
	var e error
	suboptions := subparser.Options()
	if options.SUBCOMMAND == "help" {
		e = subcmd.Invoke(suboptions)
	} else {
		e = subcmd.Invoke(sessionFactory(), suboptions)
	}
	if e != nil {
		showErrorAndExit(e)
	}
}

func main() {
	parser, e := getSubcommandsParser()
	if e != nil {
		showErrorAndExit(e)
	}
	e = parser.ParseArgs(os.Args[1:], false)
	options := parser.Options().(*BaseOptions)

	if options.Help {
		fmt.Print(parser.HelpString())
		return
	}

	if options.Version {
		fmt.Printf("Yunion API client version:\n %s\n", version.GetJsonString())
		return
	}

	ensureSessionFactory := func() *mcclient.ClientSession {
		session, err := newClientSession(options)
		if err != nil {
			showErrorAndExit(err)
		}
		return session
	}

	// enter interactive mode when not enough argument and SUBCOMMAND is empty
	if _, ok := e.(*structarg.NotEnoughArgumentsError); ok && options.SUBCOMMAND == "" {
		enterInteractiveMode(parser, ensureSessionFactory)
		return
	}

	subcmd := parser.GetSubcommand()
	subparser := subcmd.GetSubParser()
	if e != nil {
		if subparser != nil {
			fmt.Print(subparser.Usage())
		} else {
			fmt.Print(parser.Usage())
		}
		showErrorAndExit(e)
	}

	// execute subcommand in non-interactive mode
	executeSubcommand(subcmd, subparser, options, ensureSessionFactory)
}
