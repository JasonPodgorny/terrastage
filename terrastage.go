// Copyright © 2021 Jason Podgorny.
// License: https://creativecommons.org/licenses/by-nc-sa/4.0/

// The terrastage command is intended to allow you to stage files like terragrunt would
// Have but without executing terraform.   This allows you to easily tie terragrunt into
// any tool that was created to interface with native terraform.
// Some Examples:
//    - This ties into the tfcloud vcs process nicely.
//    - Makes it easy to use terraform tasks for Azure Devops Pipelines

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const OPT_TERRAGRUNT_SOURCE = "terragrunt-source"
const OPT_TERRAGRUNT_SOURCE_UPDATE = "terragrunt-source-update"

const CMD_INIT_FROM_MODULE = "init-from-module"

func main() {

	// Set Up Logging
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime)

	// Set Flag Arguments And Parse Inputs
	stagedir := flag.String("stagedir", "c:/temp/infra-stage", "Directory To Stage To")
	workdir := flag.String("workdir", ".", "Working Directory For Expression")
	subdirvar := flag.String("subdirvar", "module_path", "Variable For Subdirectory Within Stage Directory")
	fullrepo := flag.Bool("fullrepo", false, "Download Full Repo Directory Like Terragrunt Normally Would")
	verbose := flag.Bool("verbose", false, "Verbose Outputs")
	flag.Parse()

	// Get Leftover Arguments After Flag Parsing.
	extraArgs := flag.Args()

	// If There Are Extra Arguments Beyond Flags, Inputs Were Formatted Improperly
	// Print Usage/Defaults And Exit
	if len(extraArgs) > 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Add Trailing Slash To Working Directory
	*workdir = filepath.Join(*workdir, "terragrunt.hcl")

	// Log Working Directory If Output Is Verbose
	if *verbose {
		infoLog.Printf("Workdir: \n%s\n\n", *workdir)
	}

	// Log Stage Directory If Output Is Verbose
	if *verbose {
		infoLog.Printf("Stage Dir: \n%s\n\n", *stagedir)
	}

	// Log Stage Subdir Variable If Output Is Verbose
	if *verbose {
		infoLog.Printf("Stage Subdir Variable: \n%s\n\n", *subdirvar)
	}

	// Set Up Default Set Of Terragrunt Options
	// If Errors, Log And Exit
	terragruntOptions, err := options.NewTerragruntOptions(*workdir)
	if err != nil {
		errorLog.Fatalf("Get Options Had The Following Errors: %s", err)
	}

	// Set Download Dir To Staging Dir
	terragruntOptions.DownloadDir = *stagedir

	// Change Logger To Refer To Terrastage
	terragruntOptions.Logger = log.New(os.Stderr, "[terrastage] ", log.LstdFlags)

	// Parse Environment Variables And Add To Terragrunt Options
	terragruntOptions.Env = parseEnvironmentVariables(os.Environ())

	// Read Terragrunt Config File
	terragruntConfig, err := config.ReadTerragruntConfig(terragruntOptions)
	if err != nil {
		errorLog.Printf("Read Terragrunt Config Had The Following Errors: %s", err)
	}

	// See If Source URL Is Included In Terragrunt Config, If So Process That Source
	updatedTerragruntOptions := terragruntOptions
	if sourceUrl := getTerraformSourceUrl(terragruntOptions, terragruntConfig); sourceUrl != "" {

		// Get The Staging Subdirectory Variable From An Input Variable In The Terragrunt Config.
		// The Variable To Be Used Can Be Specified On The Command Line And Defaults To module_path.
		// In Our Environment This Variable To The Path Relative To Include [path_relative_to_include()]
		// This Is So That The Directory Structure Mirrors The Directory Structure Of The Source Relative
		// To The Include.   Other Strategies Are Possible, And Using A Variable From Terragrunt Inputs
		// Makes This Extremely Flexible
		stageSubDir := terragruntConfig.Inputs[*subdirvar].(string)
		// Log Stage Subdir If Output Is Verbose
		if *verbose {
			infoLog.Printf("Stage Subdir From Variable: \n%s\n\n", stageSubDir)
		}

		// Unless We Indicate To Download Full Repo, Download Just Single Module Folder
		if !*fullrepo {
			// Get rid of double / in source so it only downloads source directory and not full repo
			sourceUrl = strings.ReplaceAll(sourceUrl, "//", "/")
		}

		// Download Using Custom Download Function
		updatedTerragruntOptions, err = customDownloadTerraformSource(sourceUrl, stageSubDir, terragruntOptions, terragruntConfig)
		if err != nil {
			errorLog.Printf("Download Terraform Source Had The Following Errors: %s", err)
		}

	}

	// Change Logger To Refer To Changed Logger, For Some Reason This Reverts Back
	// Need To Look Into Reason In Code, This Is a Quick Fix
	updatedTerragruntOptions.Logger = terragruntOptions.Logger

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	for _, config := range terragruntConfig.GenerateConfigs {
		if err := codegen.WriteToFile(updatedTerragruntOptions.Logger, updatedTerragruntOptions.WorkingDir, config); err != nil {
			errorLog.Printf("Generate Configs Had The Following Errors: %s", err)
		}
	}
	if terragruntConfig.RemoteState != nil && terragruntConfig.RemoteState.Generate != nil {
		if err := terragruntConfig.RemoteState.GenerateTerraformCode(updatedTerragruntOptions); err != nil {
			errorLog.Printf("Generate Terraform Code Had The Following Errors: %s", err)
		}
	}

	// If Terragrunt Remote State Options Are Set, Use These To Generate A Backend.Config File In The Stage Directory
	// Terraform Can Then Be Initialized In This Directory With:   terraform init -backend-config "backend.config"
	if terragruntConfig.RemoteState != nil {
		if err := checkTerraformCodeDefinesBackend(updatedTerragruntOptions, terragruntConfig.RemoteState.Backend); err != nil {
			errorLog.Printf("Check Teraform Code Had The Following Errors: %s", err)
		}

		backendConfigFile := "backend.config"
		fileName := filepath.Join(updatedTerragruntOptions.WorkingDir, backendConfigFile)
		terragruntOptions.Logger.Printf(
			"Generating backend config file %s in working dir %s",
			backendConfigFile,
			updatedTerragruntOptions.WorkingDir,
		)

		// Get Remote State Cli Arguments
		remoteStateCliArgs := terragruntConfig.RemoteState.ToTerraformInitArgs()

		// Turn Cli Args Into Byte Slice For backend.config file
		backendConfigContents := make([]byte, 0)
		for _, line := range remoteStateCliArgs {
			newLine := fmt.Sprintf("%s=\"%s\"\n", strings.Split(line, "=")[1], strings.Split(line, "=")[2])
			backendConfigContents = append(backendConfigContents, []byte(newLine)...)
		}

		// Write Byte Slice To Backend Config File
		if err := ioutil.WriteFile(fileName, backendConfigContents, os.FileMode(int(0600))); err != nil {
			errorLog.Printf("Write backend.config Had The Following Errors: %s", err)
		}

	}

	// Write TFVARs File To The Staging Directory.
	// This Uses The Function That Terragrunt Debug Uses, The Log Messages
	// Are Updated To Indicate This Is A Stage And Not A Debug.
	err = writeTerragruntDebugFile(updatedTerragruntOptions, terragruntConfig)
	if err != nil {
		errorLog.Printf("Write TFVARS Had The Following Errors: %s", err)
	}

}

// Got This Function From Terragrunt Options. It Was Not Exported So Added To This Package
func parseEnvironmentVariables(environment []string) map[string]string {
	environmentMap := make(map[string]string)

	for i := 0; i < len(environment); i++ {
		variableSplit := strings.SplitN(environment[i], "=", 2)

		if len(variableSplit) == 2 {
			environmentMap[strings.TrimSpace(variableSplit[0])] = variableSplit[1]
		}
	}

	return environmentMap
}

//  Had To Grab a Function From The Terragrunt Remote Package That Wasn't Exported.
type BackendNotDefined struct {
	Opts        *options.TerragruntOptions
	BackendType string
}

func (err BackendNotDefined) Error() string {
	return fmt.Sprintf("Found remote_state settings in %s but no backend block in the Terraform code in %s. You must define a backend block (it can be empty!) in your Terraform code or your remote state settings will have no effect! It should look something like this:\n\nterraform {\n  backend \"%s\" {}\n}\n\n", err.Opts.TerragruntConfigPath, err.Opts.WorkingDir, err.BackendType)
}

func checkTerraformCodeDefinesBackend(terragruntOptions *options.TerragruntOptions, backendType string) error {
	terraformBackendRegexp, err := regexp.Compile(fmt.Sprintf(`backend[[:blank:]]+"%s"`, backendType))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	definesBackend, err := util.Grep(terraformBackendRegexp, fmt.Sprintf("%s/**/*.tf", terragruntOptions.WorkingDir))
	if err != nil {
		return err
	}
	if definesBackend {
		return nil
	}

	terraformJSONBackendRegexp, err := regexp.Compile(fmt.Sprintf(`(?m)"backend":[[:space:]]*{[[:space:]]*"%s"`, backendType))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	definesJSONBackend, err := util.Grep(terraformJSONBackendRegexp, fmt.Sprintf("%s/**/*.tf.json", terragruntOptions.WorkingDir))
	if err != nil {
		return err
	}
	if definesJSONBackend {
		return nil
	}

	return errors.WithStackTrace(BackendNotDefined{Opts: terragruntOptions, BackendType: backendType})
}
