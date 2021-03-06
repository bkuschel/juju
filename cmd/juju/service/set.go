// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewSetCommand returns a command used to set service attributes.
func NewSetCommand() cmd.Command {
	return modelcmd.Wrap(&setCommand{})
}

// setCommand updates the configuration of a service.
type setCommand struct {
	modelcmd.ModelCommandBase
	ServiceName     string
	SettingsStrings map[string]string
	Options         []string
	SettingsYAML    cmd.FileVar
	SetDefault      bool
	serviceApi      serviceAPI
}

var usageSetConfigSummary = `
Sets configuration options for a service.`[1:]

var usageSetConfigDetails = `
Charms may, and frequently do, expose a number of configuration settings
for a service to the user. These can be set at deploy time, but may be set
at any time by using the `[1:] + "`juju set-config`" + ` command. The actual options
vary per charm (you can check the charm documentation, or use ` + "`juju get-\nconfig`" +
	` to check which options may be set).
If ‘value’ begins with the ‘@’ character, it is interpreted as a filename
and the actual value is read from it. The maximum size of the filename is
5M.
Values may be any UTF-8 encoded string. UTF-8 is accepted on the command
line and in referenced files.
See ` + "`juju status`" + ` for service names.

Examples:
    juju set-config mysql dataset-size=80% backup_dir=/vol1/mysql/backups
    juju set-config apache2 --model mymodel --config /home/ubuntu/mysql.yaml

See also: 
    get-config
    deploy
    status`

const maxValueSize = 5242880

// Info implements Command.Info.
func (c *setCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-config",
		Args:    "<service name> <service key>=<value> ...",
		Purpose: usageSetConfigSummary,
		Doc:     usageSetConfigDetails,
		Aliases: []string{"set-configs"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *setCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.SettingsYAML, "config", "path to yaml-formatted service config")
	f.BoolVar(&c.SetDefault, "to-default", false, "set service option values to default")
}

// Init implements Command.Init.
func (c *setCommand) Init(args []string) error {
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no service name specified")
	}
	if c.SettingsYAML.Path != "" && len(args) > 1 {
		return errors.New("cannot specify --config when using key=value arguments")
	}
	c.ServiceName = args[0]
	if c.SetDefault {
		c.Options = args[1:]
		if len(c.Options) == 0 {
			return errors.New("no configuration options specified")
		}
		return nil
	}
	settings, err := keyvalues.Parse(args[1:], true)
	if err != nil {
		return err
	}
	c.SettingsStrings = settings
	return nil
}

// serviceAPI defines the methods on the client API
// that the service set command calls.
type serviceAPI interface {
	Close() error
	Update(args params.ServiceUpdate) error
	Get(service string) (*params.ServiceGetResults, error)
	Set(service string, options map[string]string) error
	Unset(service string, options []string) error
}

func (c *setCommand) getServiceAPI() (serviceAPI, error) {
	if c.serviceApi != nil {
		return c.serviceApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service.NewClient(root), nil
}

// Run updates the configuration of a service.
func (c *setCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getServiceAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	if c.SettingsYAML.Path != "" {
		b, err := c.SettingsYAML.Read(ctx)
		if err != nil {
			return err
		}
		return block.ProcessBlockedError(apiclient.Update(params.ServiceUpdate{
			ServiceName:  c.ServiceName,
			SettingsYAML: string(b),
		}), block.BlockChange)
	} else if c.SetDefault {
		return block.ProcessBlockedError(apiclient.Unset(c.ServiceName, c.Options), block.BlockChange)
	} else if len(c.SettingsStrings) == 0 {
		return nil
	}
	settings := map[string]string{}
	for k, v := range c.SettingsStrings {
		//empty string is also valid as a setting value
		if v == "" {
			settings[k] = v
			continue
		}

		if v[0] != '@' {
			if !utf8.ValidString(v) {
				return fmt.Errorf("value for option %q contains non-UTF-8 sequences", k)
			}
			settings[k] = v
			continue
		}
		nv, err := readValue(ctx, v[1:])
		if err != nil {
			return err
		}
		if !utf8.ValidString(nv) {
			return fmt.Errorf("value for option %q contains non-UTF-8 sequences", k)
		}
		settings[k] = nv
	}

	result, err := apiclient.Get(c.ServiceName)
	if err != nil {
		return err
	}

	for k, v := range settings {
		configValue := result.Config[k]

		configValueMap, ok := configValue.(map[string]interface{})
		if ok {
			// convert the value to string and compare
			if fmt.Sprintf("%v", configValueMap["value"]) == v {
				logger.Warningf("the configuration setting %q already has the value %q", k, v)
			}
		}
	}

	return block.ProcessBlockedError(apiclient.Set(c.ServiceName, settings), block.BlockChange)
}

// readValue reads the value of an option out of the named file.
// An empty content is valid, like in parsing the options. The upper
// size is 5M.
func readValue(ctx *cmd.Context, filename string) (string, error) {
	absFilename := ctx.AbsPath(filename)
	fi, err := os.Stat(absFilename)
	if err != nil {
		return "", fmt.Errorf("cannot read option from file %q: %v", filename, err)
	}
	if fi.Size() > maxValueSize {
		return "", fmt.Errorf("size of option file is larger than 5M")
	}
	content, err := ioutil.ReadFile(ctx.AbsPath(filename))
	if err != nil {
		return "", fmt.Errorf("cannot read option from file %q: %v", filename, err)
	}
	return string(content), nil
}
