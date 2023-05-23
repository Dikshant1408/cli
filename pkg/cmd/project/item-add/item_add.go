package itemadd

import (
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/format"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type addItemOpts struct {
	userOwner string
	orgOwner  string
	number    int32
	itemURL   string
	projectID string
	itemID    string
	format    string
}

type addItemConfig struct {
	tp     *tableprinter.TablePrinter
	client *api.GraphQLClient
	opts   addItemOpts
}

type addProjectItemMutation struct {
	CreateProjectItem struct {
		ProjectV2Item queries.ProjectItem `graphql:"item"`
	} `graphql:"addProjectV2ItemById(input:$input)"`
}

func NewCmdAddItem(f *cmdutil.Factory, runF func(config addItemConfig) error) *cobra.Command {
	opts := addItemOpts{}
	addItemCmd := &cobra.Command{
		Short: "Add a pull request or an issue to a project",
		Use:   "item-add [<number>]",
		Example: heredoc.Doc(`
			# add an item to monalisa's project "1"
			gh project item-add 1 --user monalisa --url https://github.com/monalisa/myproject/issues/23
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"only one of `--user` or `--org` may be used",
				opts.userOwner != "",
				opts.orgOwner != "",
			); err != nil {
				return err
			}

			client, err := queries.NewClient()
			if err != nil {
				return err
			}

			if len(args) == 1 {
				num, err := strconv.ParseInt(args[0], 10, 32)
				if err != nil {
					return cmdutil.FlagErrorf("invalid number: %v", args[0])
				}
				opts.number = int32(num)
			}

			t := tableprinter.New(f.IOStreams)
			config := addItemConfig{
				tp:     t,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}
			return runAddItem(config)
		},
	}

	addItemCmd.Flags().StringVar(&opts.userOwner, "user", "", "Login of the user owner. Use \"@me\" for the current user.")
	addItemCmd.Flags().StringVar(&opts.orgOwner, "org", "", "Login of the organization owner")
	addItemCmd.Flags().StringVar(&opts.itemURL, "url", "", "URL of the issue or pull request to add to the project")
	cmdutil.StringEnumFlag(addItemCmd, &opts.format, "format", "", "", []string{"json"}, "Output format")

	_ = addItemCmd.MarkFlagRequired("url")

	return addItemCmd
}

func runAddItem(config addItemConfig) error {
	owner, err := queries.NewOwner(config.client, config.opts.userOwner, config.opts.orgOwner)
	if err != nil {
		return err
	}

	project, err := queries.NewProject(config.client, owner, config.opts.number, false)
	if err != nil {
		return err
	}
	config.opts.projectID = project.ID

	itemID, err := queries.IssueOrPullRequestID(config.client, config.opts.itemURL)
	if err != nil {
		return err
	}

	config.opts.itemID = itemID

	query, variables := addItemArgs(config)
	err = config.client.Mutate("AddItem", query, variables)
	if err != nil {
		return err
	}

	if config.opts.format == "json" {
		return printJSON(config, query.CreateProjectItem.ProjectV2Item)
	}

	return printResults(config, query.CreateProjectItem.ProjectV2Item)

}

func addItemArgs(config addItemConfig) (*addProjectItemMutation, map[string]interface{}) {
	return &addProjectItemMutation{}, map[string]interface{}{
		"input": githubv4.AddProjectV2ItemByIdInput{
			ProjectID: githubv4.ID(config.opts.projectID),
			ContentID: githubv4.ID(config.opts.itemID),
		},
	}
}

func printResults(config addItemConfig, item queries.ProjectItem) error {
	// using table printer here for consistency in case it ends up being needed in the future
	config.tp.AddField("Added item")
	config.tp.EndRow()
	return config.tp.Render()
}

func printJSON(config addItemConfig, item queries.ProjectItem) error {
	b, err := format.JSONProjectItem(item)
	if err != nil {
		return err
	}
	config.tp.AddField(string(b))
	return config.tp.Render()
}
