package commands

import (
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/tenderly/tenderly-cli/buidler"
	"github.com/tenderly/tenderly-cli/config"
	"github.com/tenderly/tenderly-cli/hardhat"
	"github.com/tenderly/tenderly-cli/openzeppelin"
	"github.com/tenderly/tenderly-cli/providers"
	"github.com/tenderly/tenderly-cli/truffle"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/tenderly/tenderly-cli/model"
	"github.com/tenderly/tenderly-cli/rest"
	"github.com/tenderly/tenderly-cli/rest/call"
	"github.com/tenderly/tenderly-cli/rest/payloads"
	"github.com/tenderly/tenderly-cli/userError"
)

func newRest() *rest.Rest {
	return rest.NewRest(
		call.NewAuthCalls(),
		call.NewUserCalls(),
		call.NewProjectCalls(),
		call.NewContractCalls(),
		call.NewExportCalls(),
		call.NewNetworkCalls(),
	)
}

var deploymentProvider providers.DeploymentProvider

func extractNetworkIDs(networkIDs string) []string {
	if networkIDs == "" {
		return nil
	}

	if !strings.Contains(networkIDs, ",") {
		return []string{networkIDs}
	}

	return strings.Split(
		strings.ReplaceAll(networkIDs, " ", ""),
		",",
	)
}

func promptExportNetwork() string {
	prompt := promptui.Prompt{
		Label: "Choose the name for the exported network",
		Validate: func(input string) error {
			if len(input) == 0 {
				return errors.New("please enter the exported network name")
			}

			return nil
		},
	}

	result, err := prompt.Run()

	if err != nil {
		userError.LogErrorf("prompt export network failed: %s", err)
		os.Exit(1)
	}

	return result
}

func getProjectFromFlag(projectName string, projects []*model.Project, rest *rest.Rest) *model.Project {
	if projectName == "" {
		return nil
	}

	for _, project := range projects {
		if project.Name == projectName {
			return project
		}
	}

	if !createProject {
		return nil
	}

	projectResponse, err := rest.Project.CreateProject(
		payloads.ProjectRequest{
			Name: projectName,
		})
	if err != nil {
		userError.LogErrorf("failed creating project: %s",
			userError.NewUserError(
				err,
				"Creating the new project failed. This can happen if you are running an older version of the Tenderly CLI.",
			),
		)

		CheckVersion(true, true)

		os.Exit(1)
	}
	if projectResponse.Error != nil {
		userError.LogErrorf("create project call: %s", projectResponse.Error)
		os.Exit(1)
	}

	return projectResponse.Project
}

func promptProjectSelect(projects []*model.Project, rest *rest.Rest) *model.Project {
	var projectNames []string
	projectNames = append(projectNames, "Create new project")
	for _, project := range projects {
		var label string
		if !project.IsShared {
			label = project.Name
		} else {
			if project.Permissions == nil || !project.Permissions.AddContract {
				continue
			}
			label = fmt.Sprintf("%s (shared project)", project.Name)
		}

		projectNames = append(projectNames, label)
	}

	promptProjects := promptui.Select{
		Label: "Select Project",
		Items: projectNames,
	}

	index, _, err := promptProjects.Run()
	if err != nil {
		userError.LogErrorf("prompt project failed: %s", err)
		os.Exit(1)
	}

	if index == 0 {
		name, err := promptDefault("Project")
		if err != nil {
			userError.LogErrorf("prompt project name failed: %s", err)
			os.Exit(1)
		}

		projectResponse, err := rest.Project.CreateProject(
			payloads.ProjectRequest{
				Name: name,
			})
		if err != nil {
			userError.LogErrorf("failed creating project: %s",
				userError.NewUserError(
					err,
					"Creating the new project failed. This can happen if you are running an older version of the Tenderly CLI.",
				),
			)

			CheckVersion(true, true)

			os.Exit(1)
		}
		if projectResponse.Error != nil {
			userError.LogErrorf("create project call: %s", projectResponse.Error)
			os.Exit(1)
		}

		return projectResponse.Project
	}

	return projects[index-1]
}

func promptRpcAddress() string {
	prompt := promptui.Prompt{
		Label: "Enter rpc address (default: 127.0.0.1:8545)",
	}

	result, err := prompt.Run()

	if err != nil {
		userError.LogErrorf("prompt rpc address failed: %s", err)
		os.Exit(1)
	}

	if result == "" {
		result = "127.0.0.1:8545"
	}

	return result
}

func promptForkedNetwork(forkedNetworkNames []string) string {
	promptNetworks := promptui.Select{
		Label: "If you are forking a public network, please define which one",
		Items: forkedNetworkNames,
	}

	index, _, err := promptNetworks.Run()

	if err != nil {
		userError.LogErrorf("prompt forked network failed: %s", err)
		os.Exit(1)
	}

	if index == 0 {
		return ""
	}

	return forkedNetworkNames[index]
}

func promptProviderSelect(deploymentProviders []providers.DeploymentProviderName) providers.DeploymentProviderName {
	promptProviders := promptui.Select{
		Label: "Select Provider",
		Items: deploymentProviders,
	}

	index, _, err := promptProviders.Run()
	if err != nil {
		userError.LogErrorf("prompt provider failed: %s", err)
		os.Exit(1)
	}

	return deploymentProviders[index]
}

func initProvider() {
	trufflePath := filepath.Join(config.ProjectDirectory, truffle.NewTruffleConfigFile)
	openZeppelinPath := filepath.Join(config.ProjectDirectory, openzeppelin.OpenzeppelinConfigFile)
	oldTrufflePath := filepath.Join(config.ProjectDirectory, truffle.OldTruffleConfigFile)
	buidlerPath := filepath.Join(config.ProjectDirectory, buidler.BuidlerConfigFile)
	hardhatPath := filepath.Join(config.ProjectDirectory, hardhat.HardhatConfigFile)

	var provider providers.DeploymentProviderName

	provider = providers.DeploymentProviderName(config.MaybeGetString(config.Provider))

	var promptProviders []providers.DeploymentProviderName

	//If both config files exist, prompt user to choose
	if provider == "" || resetProvider {
		if _, err := os.Stat(openZeppelinPath); err == nil {
			promptProviders = append(promptProviders, providers.OpenZeppelinDeploymentProvider)
		}
		if _, err := os.Stat(trufflePath); err == nil {
			promptProviders = append(promptProviders, providers.TruffleDeploymentProvider)
		} else if _, err := os.Stat(oldTrufflePath); err == nil {
			promptProviders = append(promptProviders, providers.TruffleDeploymentProvider)
		}
		if _, err := os.Stat(buidlerPath); err == nil {
			promptProviders = append(promptProviders, providers.BuidlerDeploymentProvider)
		}
		if _, err := os.Stat(buidlerPath); err == nil {
			promptProviders = append(promptProviders, providers.HardhatDeploymentProvider)
		}
	}

	if len(promptProviders) > 1 {
		provider = promptProviderSelect(promptProviders)
	}

	if provider != "" {
		config.SetProjectConfig(config.Provider, provider)
		WriteProjectConfig()
	}

	logrus.Debugf("Trying OpenZeppelin config path: %s", openZeppelinPath)
	if provider == providers.OpenZeppelinDeploymentProvider || provider == "" {

		_, err := os.Stat(openZeppelinPath)

		if err == nil {
			deploymentProvider = openzeppelin.NewDeploymentProvider()
			return
		}

		logrus.Debugf(
			fmt.Sprintf("unable to fetch config\n%s",
				" Couldn't read OpenZeppelin config file"),
		)
	}

	logrus.Debugf("couldn't read new OpenzeppelinConfig config file")

	logrus.Debugf("Trying buidler config path: %s", buidlerPath)

	if provider == providers.BuidlerDeploymentProvider || provider == "" {
		_, err := os.Stat(buidlerPath)

		if err == nil {
			deploymentProvider = buidler.NewDeploymentProvider()

			if deploymentProvider == nil {
				logrus.Error("Error initializing buidler")
			}

			return
		}

		logrus.Debugf(
			fmt.Sprintf("unable to fetch config\n%s",
				" Couldn't read Buidler config file"),
		)
	}

	logrus.Debugf("couldn't read new Buidler config file")

	logrus.Debug("Trying hardhat config path: %s", hardhatPath)

	if provider == providers.HardhatDeploymentProvider || provider == "" {
		_, err := os.Stat(hardhatPath)

		if err == nil {
			deploymentProvider = hardhat.NewDeploymentProvider()

			if deploymentProvider == nil {
				logrus.Error("Error initializing hardhat")
			}

			return
		}

		logrus.Debugf(
			fmt.Sprintf("unable to fetch config\n%s",
				" Couldn't read Hardhat config file"),
		)
	}

	logrus.Debugf("Trying truffle config path: %s", trufflePath)

	_, err := os.Stat(trufflePath)

	if err == nil {
		deploymentProvider = truffle.NewDeploymentProvider()
		return
	}

	if !os.IsNotExist(err) {
		logrus.Debugf(
			fmt.Sprintf("unable to fetch config\n%s",
				"Couldn't read Truffle config file"),
		)
		os.Exit(1)
	}

	logrus.Debugf("couldn't read new truffle config file: %s", err)

	logrus.Debugf("Trying old truffle config path: %s", trufflePath)

	_, err = os.Stat(oldTrufflePath)

	if err == nil {
		deploymentProvider = truffle.NewDeploymentProvider()
		return
	}

	logrus.Debugf(
		fmt.Sprintf("unable to fetch config\n%s",
			"Couldn't read old Truffle config file"),
	)
}

func GetConfigPayload(providerConfig *providers.Config) *payloads.Config {
	if providerConfig.ConfigType == truffle.NewTruffleConfigFile && providerConfig.Compilers != nil {
		return payloads.ParseNewTruffleConfig(providerConfig.Compilers)
	}

	if providerConfig.ConfigType == truffle.OldTruffleConfigFile {
		if providerConfig.Solc != nil {
			return payloads.ParseOldTruffleConfig(providerConfig.Solc)
		} else if providerConfig.Compilers != nil {
			return payloads.ParseNewTruffleConfig(providerConfig.Compilers)
		}
	}
	if providerConfig.ConfigType == openzeppelin.OpenzeppelinConfigFile && providerConfig.Compilers != nil {
		return payloads.ParseOpenZeppelinConfig(providerConfig.Compilers)
	}

	if providerConfig.ConfigType == buidler.BuidlerConfigFile && providerConfig.Compilers != nil {
		return payloads.ParseBuidlerConfig(providerConfig.Compilers)
	}

	if providerConfig.ConfigType == hardhat.HardhatConfigFile && providerConfig.Compilers != nil {
		return payloads.ParseHardhatConfig(providerConfig.Compilers)
	}

	return nil
}
