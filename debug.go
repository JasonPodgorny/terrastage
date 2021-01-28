package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

//const TerragruntTFVarsFile = "terragrunt-debug.tfvars.json"

// Updated File Name For TFVars
const TerragruntTFVarsFile = "test.auto.tfvars.json"

// writeTerragruntDebugFile will create a tfvars file that can be used to invoke the terraform module in the same way
// that terragrunt invokes the module, so that you can debug issues with the terragrunt config.
func writeTerragruntDebugFile(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terragruntOptions.Logger.Printf(
		"Generating TFVARS file %s in working dir %s",
		TerragruntTFVarsFile,
		terragruntOptions.WorkingDir,
	)

	variables, err := terraformModuleVariables(terragruntOptions)
	if err != nil {
		return err
	}
	util.Debugf(terragruntOptions.Logger, "The following variables were detected in the terraform module:")
	util.Debugf(terragruntOptions.Logger, "%v", variables)

	fileContents, err := terragruntDebugFileContents(terragruntOptions, terragruntConfig, variables)
	if err != nil {
		return err
	}

	// configFolder := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	// fileName := filepath.Join(configFolder, TerragruntTFVarsFile)

	// Updated Location For File Name.
	// Points To Staging Directory / Staging Subdirectory
	fileName := filepath.Join(terragruntOptions.WorkingDir, TerragruntTFVarsFile)

	if err := ioutil.WriteFile(fileName, fileContents, os.FileMode(int(0600))); err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Printf("Variables passed to terraform are located in \"%s\"", fileName)
	terragruntOptions.Logger.Printf("Run this command to replicate how terraform was invoked:")
	terragruntOptions.Logger.Printf(
		"\tterraform %s -var-file=\"%s\" \"%s\"",
		strings.Join(terragruntOptions.TerraformCliArgs, " "),
		fileName,
		terragruntOptions.WorkingDir,
	)
	return nil
}

// terragruntDebugFileContents will return a tfvars file in json format of all the terragrunt rendered variables values
// that should be set to invoke the terraform module in the same way as terragrunt. Note that this will only include the
// values of variables that are actually defined in the module.
func terragruntDebugFileContents(
	terragruntOptions *options.TerragruntOptions,
	terragruntConfig *config.TerragruntConfig,
	moduleVariables []string,
) ([]byte, error) {
	envVars := map[string]string{}
	if terragruntOptions.Env != nil {
		envVars = terragruntOptions.Env
	}

	jsonValuesByKey := make(map[string]interface{})
	for varName, varValue := range terragruntConfig.Inputs {
		nameAsEnvVar := fmt.Sprintf("TF_VAR_%s", varName)
		_, varIsInEnv := envVars[nameAsEnvVar]
		varIsDefined := util.ListContainsElement(moduleVariables, varName)

		// Only add to the file if the explicit env var does NOT exist and the variable is defined in the module.
		// We must do this in order to avoid overriding the env var when the user follows up with a direct invocation to
		// terraform using this file (due to the order in which terraform resolves config sources).
		if !varIsInEnv && varIsDefined {
			jsonValuesByKey[varName] = varValue
		} else if varIsInEnv {
			util.Debugf(
				terragruntOptions.Logger,
				"WARN: The variable %s was omitted from the debug file because the env var %s is already set.",
				varName, nameAsEnvVar,
			)
		} else if !varIsDefined {
			util.Debugf(
				terragruntOptions.Logger,
				"WARN: The variable %s was omitted because it is not defined in the terraform module.",
				varName,
			)
		}
	}
	jsonContent, err := json.MarshalIndent(jsonValuesByKey, "", "  ")
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return jsonContent, nil
}

// terraformModuleVariables will return all the variables defined in the downloaded terraform modules, taking into
// account all the generated sources.
func terraformModuleVariables(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	modulePath := terragruntOptions.WorkingDir
	module, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, errors.WithStackTrace(diags)
	}

	variables := []string{}
	for _, variable := range module.Variables {
		variables = append(variables, variable.Name)
	}
	return variables, nil
}
