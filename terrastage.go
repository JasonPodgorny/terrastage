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

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const OPT_TERRAGRUNT_SOURCE = "terragrunt-source"
const OPT_TERRAGRUNT_SOURCE_UPDATE = "terragrunt-source-update"

const CMD_INIT_FROM_MODULE = "init-from-module"

func main() {

	// Set Flag Arguments And Parse Inputs
	stagedir := flag.String("stagedir", ".", "Directory To Stage To")
	workdir := flag.String("workdir", ".", "Working Directory For Expression")
	subdirvar := flag.String("subdirvar", "module_path", "Variable For Subdirectory Within Stage Directory")
	//fullrepo := flag.Bool("fullrepo", false, "Download Full Repo Directory Like Terragrunt Normally Would")
	verbose := flag.Bool("verbose", false, "Verbose Outputs")
	debug := flag.Bool("debug", false, "Debug Outputs")
	flag.Parse()

	// Get Leftover Arguments After Flag Parsing.
	extraArgs := flag.Args()

	// If There Are Extra Arguments Beyond Flags, Inputs Were Formatted Improperly
	// Print Usage/Defaults And Exit
	if len(extraArgs) > 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Set Up Default Set Of Terragrunt Options
	terragruntOptions := options.NewTerragruntOptions()

	// Set Log Level To Debug If -debug Flag Is Set
	if *debug {
		logLevel := util.ParseLogLevel("debug")
		terragruntOptions.Logger = util.CreateLogEntry("", logLevel)
	}

	// If Workdir Is . Then Get Current Path
	if *workdir == "." {
		path, err := os.Getwd()
		if err != nil {
			log.Println(err)
		}
		*workdir = path
	}

	// Add Trailing Slash To Working Directory
	*workdir = *workdir + string(os.PathSeparator)

	// If Stagedir Is . Then Get Current Path
	if *stagedir == "." {
		path, err := os.Getwd()
		if err != nil {
			log.Println(err)
		}
		*stagedir = path
		*stagedir = *stagedir + string(os.PathSeparator) + ".terrastage"
	}

	// Add Trailing Slash To Stage Directory
	*stagedir = *stagedir + string(os.PathSeparator)

	// Set Config Path
	configPath := filepath.Join(*workdir, "terragrunt.hcl")

	// Log Working Directory, Stage Directory, and Stage Subdirectory If Output Is Debug
	if *verbose || *debug {
		terragruntOptions.Logger.Infof("Workdir: %s", *workdir)
		terragruntOptions.Logger.Infof("Stage Dir: %s", *stagedir)
		terragruntOptions.Logger.Infof("Stage Subdir Variable: %s", *subdirvar)
		//infoLog.Printf("Workdir: \n%s\n\n", *workdir)
	}

	// Set Woring Dir To Working Dir
	terragruntOptions.WorkingDir = *workdir

	// Set Config Path To Config Path
	terragruntOptions.TerragruntConfigPath = configPath

	// Set Download Dir To Staging Dir
	terragruntOptions.DownloadDir = *stagedir

	// Parse Environment Variables And Add To Terragrunt Options
	terragruntOptions.Env = parseEnvironmentVariables(os.Environ())

	// Read Terragrunt Config File
	terragruntConfig, err := config.ReadTerragruntConfig(terragruntOptions)
	if err != nil {
		terragruntOptions.Logger.Errorf("Read Terragrunt Config Had The Following Errors: %s", err)
	}

	// See If Source URL Is Included In Terragrunt Config, If So Process That Source
	updatedTerragruntOptions := terragruntOptions
	sourceUrl, err := config.GetTerraformSourceUrl(terragruntOptions, terragruntConfig)
	if err != nil {
		terragruntOptions.Logger.Errorf("Get Source URL Had The Following Errors: %s", err)
	}
	if sourceUrl != "" {

		// Get The Staging Subdirectory Variable From An Input Variable In The Terragrunt Config.
		// The Variable To Be Used Can Be Specified On The Command Line And Defaults To module_path.
		// In Our Environment This Variable To The Path Relative To Include [path_relative_to_include()]
		// This Is So That The Directory Structure Mirrors The Directory Structure Of The Source Relative
		// To The Include.   Other Strategies Are Possible, And Using A Variable From Terragrunt Inputs
		// Makes This Extremely Flexible
		stageSubDir := ""
		if terragruntConfig.Inputs[*subdirvar] != nil {
			stageSubDir = terragruntConfig.Inputs[*subdirvar].(string)
		}

		// Log Stage Subdir If Output Is Verbose
		if *verbose || *debug {
			terragruntOptions.Logger.Infof("Stage Subdir From Variable: %s", stageSubDir)
		}

		// Unless We Indicate To Download Full Repo, Download Just Single Module Folder
		//if !*fullrepo {
		//Get rid of double / in source so it only downloads source directory and not full repo
		//	sourceUrl = strings.ReplaceAll(sourceUrl, "//", "/")
		//}

		// Download Using Custom Download Function
		updatedTerragruntOptions, err = customDownloadTerraformSource(sourceUrl, stageSubDir, terragruntOptions, terragruntConfig)
		if err != nil {
			terragruntOptions.Logger.Errorf("Download Terraform Source Had The Following Errors: %s", err)
		}

	}

	// Change Logger To Refer To Changed Logger, For Some Reason This Reverts Back
	// Need To Look Into Reason In Code, This Is a Quick Fix
	updatedTerragruntOptions.Logger = terragruntOptions.Logger

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	for _, config := range terragruntConfig.GenerateConfigs {
		if err := codegen.WriteToFile(updatedTerragruntOptions, updatedTerragruntOptions.WorkingDir, config); err != nil {
			terragruntOptions.Logger.Errorf("Generate Configs Had The Following Errors: %s", err)
		}
	}
	if terragruntConfig.RemoteState != nil && terragruntConfig.RemoteState.Generate != nil {
		if err := terragruntConfig.RemoteState.GenerateTerraformCode(updatedTerragruntOptions); err != nil {
			terragruntOptions.Logger.Errorf("Generate Terraform Code Had The Following Errors: %s", err)
		}
	}

	// If Terragrunt Remote State Options Are Set, Use These To Generate A Backend.Config File In The Stage Directory
	// Terraform Can Then Be Initialized In This Directory With:   terraform init -backend-config "backend.config"
	if terragruntConfig.RemoteState != nil {
		if err := checkTerraformCodeDefinesBackend(updatedTerragruntOptions, terragruntConfig.RemoteState.Backend); err != nil {
			terragruntOptions.Logger.Errorf("Check Teraform Code Had The Following Errors: %s", err)
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
			terragruntOptions.Logger.Errorf("Write backend.config Had The Following Errors: %s", err)
		}

	}

	// Write TFVARs File To The Staging Directory.
	// This Uses The Function That Terragrunt Debug Uses, The Log Messages
	// Are Updated To Indicate This Is A Stage And Not A Debug.
	err = WriteTerragruntDebugFile(updatedTerragruntOptions, terragruntConfig)
	if err != nil {
		terragruntOptions.Logger.Errorf("Write TFVARS Had The Following Errors: %s", err)
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
