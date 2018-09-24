/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package verification

// Waits for a given cluster to complete installing

import (
	"fmt"
	"os"

	"go/ast"
	"go/parser"
	"go/token"
	"gopkg.in/yaml.v2"
	"io/ioutil"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// VerifyImportsOptions contains the options for verifying go imports
type VerifyImportsOptions struct {
	GoFile     string
	ConfigFile string
	Logger     logger.FieldLogger
}

// NewVerifyImportsCommand adds a subcommand for verifying imports of a go file.
func NewVerifyImportsCommand() *cobra.Command {
	opt := &VerifyImportsOptions{}
	logLevel := "info"
	cmd := &cobra.Command{
		Use:   "verify-imports GO-FILE",
		Short: "Verify the imports of GO-FILE match requirements",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Usage()
				return
			}

			// Set log level
			level, err := logger.ParseLevel(logLevel)
			if err != nil {
				logger.WithError(err).Error("Cannot parse log level")
				os.Exit(1)
			}

			log := logger.NewEntry(&logger.Logger{
				Out: os.Stdout,
				Formatter: &logger.TextFormatter{
					FullTimestamp: true,
				},
				Hooks: make(logger.LevelHooks),
				Level: level,
			})
			opt.Logger = log

			// Run command
			opt.GoFile = args[0]
			err = opt.VerifyImports()
			if err != nil {
				opt.Logger.Errorf("Error validating %v: %v", opt.GoFile, err)
				os.Exit(1)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.ConfigFile, "config", "c", "", "Config file that holds the verification rules for imports")
	return cmd
}

// VerifyImports verifies that the imports match the required convention.
func (o *VerifyImportsOptions) VerifyImports() error {
	if _, err := os.Stat(o.GoFile); os.IsNotExist(err) {
		o.Logger.WithError(err).Error("Go File Not Found")
		return err
	}

	if _, err := os.Stat(o.ConfigFile); os.IsNotExist(err) {
		o.Logger.WithError(err).Error("Config File Not Found")
		return err
	}

	ast, err := parser.ParseFile(token.NewFileSet(), o.GoFile, nil, parser.ImportsOnly)

	if err != nil {
		o.Logger.WithError(err).Error("Failed parsing go-file")
		return err
	}

	config := &config{}
	err = config.load(o)
	if err != nil {
		return err
	}

	return verifyImports(ast.Imports, config.Rules)
}

func verifyImports(imports []*ast.ImportSpec, rules []*rule) error {
	invalidImports := []error{}

	for _, importSpec := range imports {
		for _, rule := range rules {
			var goImportName string

			if importSpec.Name != nil {
				goImportName = importSpec.Name.Name
			}

			// go imports come in with quotes around them. This is necessary for the comparison to succeed.
			ruleImportPath := "\"" + rule.ImportPath + "\""

			if !isValidImport(goImportName, importSpec.Path.Value, rule.ImportName, ruleImportPath) {
				err := fmt.Errorf("'%s %s' should be '%s %s'", goImportName, importSpec.Path.Value, rule.ImportName, ruleImportPath)

				if goImportName == "" {
					err = fmt.Errorf("'%s' should be '%s %s'", importSpec.Path.Value, rule.ImportName, ruleImportPath)
				}

				if rule.ImportName == "" {
					err = fmt.Errorf("'%s %s' should be '%s'", goImportName, importSpec.Path.Value, ruleImportPath)
				}
				invalidImports = append(invalidImports, err)
			}
		}
	}

	return utilerrors.NewAggregate(invalidImports)
}

func isValidImport(goImportName, goImportPath, ruleImportName, ruleImportPath string) bool {
	if goImportPath != ruleImportPath {
		return true // Do not check this one as it's a different path from the rule.
	}

	if ruleImportName != goImportName {
		return false // The import names don't match.
	}

	// If we make it here, then it passes all the checks.
	return true
}

type rule struct {
	ImportName string `yaml:"importName"`
	ImportPath string `yaml:"importPath"`
}

type config struct {
	Rules []*rule `yaml:"rules"`
}

func (c *config) load(options *VerifyImportsOptions) error {

	raw, err := ioutil.ReadFile(options.ConfigFile)
	if err != nil {
		options.Logger.WithError(err).Error("Failed reading config file")
		return err
	}

	err = yaml.Unmarshal(raw, c)
	if err != nil {
		options.Logger.WithError(err).Error("Failed parsing config file")
		return err
	}

	return nil
}
