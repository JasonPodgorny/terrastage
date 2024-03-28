# terrastage

## Overview
Terrastage is a utility that has been designed to make it easier for terragrunt based terraform configurations to integrate easily with tools based on native terraform.   

One popular example is [VCS Repository based workspaces](https://www.terraform.io/docs/cloud/workspaces/vcs.html) in [Terraform Cloud](https://www.terraform.io/cloud).   These workspaces only interface with native terraform code, so without a tool like terrastage you are limited to using CLI based workspaces and execution, which doesn't offer the same robust set of log information and execution capabilities of the VCS based workspaces.   

Another example is using the [Azure Pipelines Terraform Tasks](https://marketplace.visualstudio.com/items?itemName=charleszipp.azure-pipelines-tasks-terraform).  These tasks make it very easy to integrate terraform into an Azure Pipeline, but similar to terraform cloud VCS Workspaces this requires native terraform code.

The utility will allow easy integration into any tool that expects native terraform code.   It can also be used as simply a tool to help troubleshoot issues encountered in terragrunt based solutions allowing you to stage the native terraform code as terragrunt would have and then use native terrform on the fly debugging and easier problem resolution.

# How It Works

[Terragrunt](https://terragrunt.gruntwork.io/) is a popular wrapper utility for terraform based workloads that helps to organize your terraform code in a more DRY (Don't Repeat Yourself) code-oriented configuration.   When code is executed for a given directory/workspace it stages all of the terraform source and terraform configuration into a temporary area, and then executes terraform from this temporary area so that terraform itself is operating on native terraform code and is unaware that the wrapper did this staging.   In normal execution it passes inputs to this terraform run using environmental variables but also includes a [debug option](https://terragrunt.gruntwork.io/docs/features/debugging/) that creates a tfvars file so that these runs can be executed in native terraform for easier troubleshooting.

The trouble with the default behavior in terms of integrating with some native terraform tools is that there is no control over the temporary directories being staged to.  In order to avoid conflicts in the temporary area they put directory names through hashing functions.   This makes it impossible to have control over the structure of your VCS based repositories that would back Terraform Cloud, or any other similar solution.  It also poses challenges when trying to use native terraform tasks from other VCS / Pipeline based solutions.

Terrastage offers a solution to this problem by using the same terragrunt libraries, but giving the user control over where the code and tfvars files get staged to.   It only stages the files and takes no action with terraform itself.   Once the native terraform staged, control can be passed to whatever native terraform utility is being leveraged.


## ** WARNING - ESPECIALLY IF YOU COMMIT THESE FILES TO A VCS REPO**

__NOTE : A tfvars file that includes all inputs that terragrunt generates is put in the stage directory.   If you have been feeding sensitive values to terraform using these inputs they can potentially be exposed in this file.__

__This risk isn't unique to this utility, but is always a consideration when feeding input variables to terraform modules.   It is even what has caused gruntwork themselves to not roll this out as a standard feature more widely and instead limit this to the debug switch for the time being.  It's noted here since terragrunt's normal mode of operation is to feed these as environment variables to terraform when it executes it internally, and terragrunt users need to consider the implication of generating a tfvars file from those variables.__
    
__Best practice is to avoid including sensitive values in these inputs and find a solution that is more secure (Environment secrets offered by the pipeline tool of your choice, some sort of secret manager, etc.)__

__Otherwise, use temporary instances / temporary directories that are being discarded, and commit just your terragrunt configuration files and terraform code itself to VCS like is typically common practice in these environments.__



# Usage

```
Usage of terrastage:
  -stagedir string
        Directory To Stage To (default ".")
  -subdirvar string
        Variable For Subdirectory Within Stage Directory (default "module_path")
  -verbose
        Verbose Outputs
  -debug
        Debug Outputs
  -workdir string
        Working Directory For Expression (default ".")
```

## -workdir
This is the directory of the terragrunt configuration (terragrunt.hcl) that is being staged.  By default it will look in the present working directory

## -stagedir
This is the base directory the terraform code should be staged to.   By default this is c:/temp/infra-stage.

## -subdirvar
This setting points to an input variable from your terragrunt configuration that sets the subdirectory within the stage directory that should be staged to.   By consuming this from a terragrunt input variable there is a lot of flexibility in how this variable can be populated.   A common pattern is to use this along with the include block and populate the variable using the terragrunt path_relative_to_include() function, but many options are possible.

## -fullrepo
Source statements in terragrunt and terraform allow you to separate your module source from the directory within that source using two forward slashes (//).   It will download all subdirectories for everything after the double forward slash.  By default the utility will strip this double slash and only stage the files inside the full folder being specified.   This option overrides this and allows you to stage using the default terragrunt / terraform behavior.

## -verbose
A few more outputs

# Operational Details
The [Terragrunt](https://terragrunt.gruntwork.io/) libraries are used for the program.   There are a few modifications to the base program that allow control for the placement of the "temporary files" and a couple of additions for what goes into those files.  They have some excellent documentation there on the operations of terragrunt itself.    For this helper utility the following steps occur:

1.  The terragrunt options and configurations (along with includes) are read in like they normally would be in terragrunt itself.
2.  The staging location is calculated using the variables specified in the usage section rather than the default terragrunt hashed locations.
3.  The files from the terragrunt configuration working directory are downloaded to this location.
4.  The files specified in the terraform source location are downloaded into this location.
5.  The generate blocks for terragrunt are run like they normally would be, and new generated files are dropped into this location.
6.  For remote state configurations that weren't in generate blocks, a terraform file for the backend is generated and put in this location.  This file is called backend.config
7.  A tfvars file is generated using the function that is used for the terragrunt debug function.  Instead of this file being placed in the terragrunt working directory, this goes to the staging location.   This is called test.auto.tfvars.json.   (Yeah it's still called test. It's v0.1!!)

That's it.   After these steps you have native terraform code that can fit into any pipeline.

# Native Terraform Local Examples

## Terrastage File Stage

Using default options it will stage the current working directory and send to c:\temp\infra-stage

```
PS C:\temp\infra-live\dev\centralus\myterragruntmodule> terrastage.exe

[terrastage] 2021/01/28 13:48:49 Reading Terragrunt config file at terragrunt.hcl
[terrastage] 2021/01/28 13:48:49 WARNING: no double-slash (//) found in source URL C:/temp/infra-mod/modules/myterragruntmodule. Relative paths in downloaded Terraform code may not work.
[terrastage] 2021/01/28 13:48:49 Downloading Terraform configurations from file://C:/temp/infra-mod/modules/myterragruntmodule into c:/temp/infra-stage/dev/centralus/myterragruntmodule 
[terrastage] 2021/01/28 13:48:49 Copying files from . into c:/temp/infra-stage/dev/centralus/myterragruntmodule
[terrastage] 2021/01/28 13:48:49 Setting working directory to c:/temp/infra-stage/dev/centralus/myterragruntmodule
[terrastage] 2021/01/28 13:48:49 Generating backend config file backend.config in working dir c:/temp/infra-stage/dev/centralus/myterragruntmodule
[terrastage] 2021/01/28 13:48:49 Generating TFVARS file test.auto.tfvars.json in working dir c:/temp/infra-stage/dev/centralus/myterragruntmodule
[terrastage] 2021/01/28 13:48:49 Variables passed to terraform are located in "c:\temp\infra-stage\dev\centralus\myterragruntmodule\test.auto.tfvars.json"
[terrastage] 2021/01/28 13:48:49 Run this command to replicate how terraform was invoked:
[terrastage] 2021/01/28 13:48:49        terraform  -var-file="c:\temp\infra-stage\dev\centralus\myterragruntmodule\test.auto.tfvars.json" "c:/temp/infra-stage/dev/centralus/myterragruntmodule"

time=2024-03-28T07:18:40-04:00 level=info msg=Downloading Terraform configurations from file://C:/temp/infra-mod/modules/myterragruntmodule into C:/temp/infra-live/dev/centralus/myterragruntmodule/.terrastage/
time=2024-03-28T07:18:42-04:00 level=info msg=Generating backend config file backend.config in working dir C:/temp/infra-live/dev/centralus/myterragruntmodule/.terrastage/
time=2024-03-28T07:18:42-04:00 level=info msg=Generating TFVARS file test.auto.tfvars.json in working dir C:/temp/infra-live/dev/centralus/myterragruntmodule/.terrastage/
time=2024-03-28T07:18:42-04:00 level=info msg=Variables passed to terraform are located in "C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage\test.auto.tfvars.json"
time=2024-03-28T07:18:42-04:00 level=info msg=Run this command to replicate how terraform was invoked:
time=2024-03-28T07:18:42-04:00 level=info msg=  terraform -chdir="C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage"  -var-file="C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage\test.auto.tfvars.json"
```

## Terraform Init

If you are using generate blocks to generate a full backend configuration file, you can simply go to the staging directory and perform your terraform init:

```
PS C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage> terraform init

Initializing modules...
- examplemodule in ..\lib\examplemodule

Initializing the backend...

Successfully configured the backend "azurerm"! Terraform will automatically
use this backend unless the backend configuration changes.

Initializing provider plugins...
- Checking for available provider plugins...
- Downloading plugin for provider "azurerm" (hashicorp/azurerm) 2.30.0...
- Downloading plugin for provider "azuread" (hashicorp/azuread) 0.6.0...
- Downloading plugin for provider "external" (hashicorp/external) 2.0.0...

The following providers do not have any version constraints in configuration,
so the latest version was installed.

To prevent automatic upgrades to new major versions that may contain breaking
changes, it is recommended to add version = "..." constraints to the
corresponding provider blocks in configuration, with the constraint strings
suggested below.

* provider.external: version = "~> 2.0"

Terraform has been successfully initialized!

You may now begin working with Terraform. Try running "terraform plan" to see
any changes that are required for your infrastructure. All Terraform commands
should now work.

If you ever set or change modules or backend configuration for Terraform,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.
```

If you are using remote state blocks that don't use the generate feature, terragrunt normally passed those in the init phase using var statements.   A backend.config file has been created using those values so these can instead be initialized using the following command:

```
PS C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage> terraform init -backend-config="backend.config"

Initializing modules...
- examplemodule in ..\lib\examplemodule

Initializing the backend...

Successfully configured the backend "azurerm"! Terraform will automatically
use this backend unless the backend configuration changes.

Initializing provider plugins...
- Checking for available provider plugins...
- Downloading plugin for provider "azurerm" (hashicorp/azurerm) 2.30.0...
- Downloading plugin for provider "azuread" (hashicorp/azuread) 0.6.0...
- Downloading plugin for provider "external" (hashicorp/external) 2.0.0...

The following providers do not have any version constraints in configuration,
so the latest version was installed.

To prevent automatic upgrades to new major versions that may contain breaking
changes, it is recommended to add version = "..." constraints to the
corresponding provider blocks in configuration, with the constraint strings
suggested below.

* provider.external: version = "~> 2.0"

Terraform has been successfully initialized!

You may now begin working with Terraform. Try running "terraform plan" to see
any changes that are required for your infrastructure. All Terraform commands
should now work.

If you ever set or change modules or backend configuration for Terraform,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.
```

## Post Init

Once you have initialized the directory, you can now execute any native terraform commands against this folder directly.   

```
PS C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage> terraform plan
PS C:\temp\infra-live\dev\centralus\myterragruntmodule\.terrastage> terraform apply
etc...
```

# Azure Pipeline Example
```
steps:
- checkout: self

- script: |
    mkdir $(Agent.BuildDirectory)\s\temp
    $(Agent.BuildDirectory)\s\infra-live\terrastage.exe -stagedir $(Agent.BuildDirectory)\s\temp -workdir $(Agent.BuildDirectory)\s\infra-live\dev\centralus\myterragruntmodule
  displayName: 'Terragrunt Stage'

- task: TerraformInstaller@0
  displayName: 'Install Terraform 0.12.9'
  inputs:
    terraformVersion: 0.12.9

- task: TerraformCLI@0
  displayName: Terraform Init
  inputs:
    command: 'init'
    commandOptions: '-backend-config="backend.config"'
    workingDirectory: $(Agent.BuildDirectory)\s\temp\dev\centralus\myterragruntmodule
    backendType: azurerm
    backendServiceArm: 'my-msdn'
    #
    # Getting These Values From The Command Options
    # The Task Forces You To Input Variables For These
    # Once You Select AzureRM As The Back End But Doesn't
    # Really Use Them When Backend Is Read In Command Options
    #
    backendAzureRmResourceGroupName: 'dummy'
    backendAzureRmResourceGroupLocation: 'dummy'
    backendAzureRmStorageAccountName: 'dummy'
    backendAzureRmContainerName: 'dummy'
    backendAzureRmKey: dummy

- task: TerraformCLI@0
  displayName: 'Terraform Plan'
  inputs:
    command: plan
    workingDirectory: $(Agent.BuildDirectory)\s\temp\dev\centralus\myterragruntmodule
    environmentServiceName: 'my-msdn'
```
