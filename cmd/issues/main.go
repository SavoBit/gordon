package main

import (
	"fmt"
	"github.com/docker/gordon/pkg/gordon"
	"os"
	"path"
	"time"

	"github.com/urfave/cli"
	gh "github.com/crosbymichael/octokat"
	"github.com/docker/gordon/pkg/filters"
)

var (
	m             *gordon.MaintainerManager
	remote_origin = "origin"
	configPath    = path.Join(os.Getenv("HOME"), ".maintainercfg")
)

func alruCmd(c *cli.Context) error {
	lru, err := m.GetFirstIssue("open", "updated")
	if err != nil {
		gordon.Fatalf("Error getting issues: %s", err)
	}
	fmt.Printf("%v (#%d)\n", gordon.HumanDuration(time.Since(lru.UpdatedAt)), lru.Number)
	return nil
}

func repositoryInfoCmd(c *cli.Context) error {
	r, err := m.Repository()
	if err != nil {
		gordon.Fatalf("%s", err)
	}
	fmt.Printf("Name: %s\nForks: %d\nStars: %d\nIssues: %d\n", r.Name, r.Forks, r.Watchers, r.OpenIssues)
	return nil
}

//Take a specific issue. If it's taken, show a message with the overwrite optional flag
//If the user doesn't have permissions, add a comment #volunteer
func takeCmd(c *cli.Context) error {
	if c.Args().Present() {
		number := c.Args()[0]
		issue, _, err := m.GetIssue(number, false)
		if err != nil {
			gordon.Fatalf("%s", err)
		}
		user, err := m.GetGithubUser()
		if err != nil {
			gordon.Fatalf("%s", err)
		}
		if user == nil {
			gordon.Fatalf("%v", gordon.ErrNoUsernameKnown)
		}
		if issue.Assignee.Login != "" && !c.Bool("overwrite") {
			fmt.Printf("Use the flag --overwrite to take the issue from %s\n", issue.Assignee.Login)
			return nil
		}
		issue.Assignee = *user
		patchedIssue, err := m.PatchIssue(number, issue)
		if err != nil {
			gordon.Fatalf("%s", err)
		}
		if patchedIssue.Assignee.Login != user.Login {
			m.AddComment(number, "#volunteer")
			fmt.Printf("No permission to assign. You '%s' was added as #volunteer.\n", user.Login)
		} else {
			fmt.Printf("The issue %s was assigned to %s\n", number, patchedIssue.Assignee.Login)
		}
	} else {
		fmt.Printf("Please enter the issue's number\n")
	}
	return nil
}

func closeCmd(c *cli.Context) error {
	if !c.Args().Present() {
		gordon.Fatalf("Please enter the issue's number")
	}
	number := c.Args()[0]
	if err := m.Close(number); err != nil {
		gordon.Fatalf("%v", err)
	}
	fmt.Printf("Closed issue %s\n", number)
	return nil
}

func buildQuery(c *cli.Context) string {
	r, err := m.Repository()
	if err != nil {
		gordon.Fatalf("%s", err)
	}
	// standard parameters
	query := fmt.Sprintf("q=%s+repo:%s", c.Args()[0], r.FullName)
	state := c.String("state")
	if state == "" {
		state = "open"
	}
	query += fmt.Sprintf("+state:%s", state)
	// optional parameters
	var optionalParameters = []string{
		"author",
		"assignee",
		"mentions",
		"commenter",
		"involves",
		"labels"}

	for i := 0; i < len(optionalParameters); i++ {
		param := optionalParameters[i]
		value := c.String(param)
		if value != "" {
			query += fmt.Sprintf("+%s:%s", param, value)
		}
	}
	return query
}

//Search for issues. You add some restrictions to the query. such:
// authors, assignee, state, etc. Check the command help for more options.
func searchCmd(c *cli.Context) error {
	if c.Args().Present() {
		issues, err := m.GetIssuesFound(buildQuery(c))
		if err != nil {
			gordon.Fatalf("%s", err)
		}
		fmt.Printf("%c[2K\r", 27)
		gordon.DisplayIssues(c, issues, c.Bool("no-trunc"))
	} else {
		fmt.Printf("Please enter a search term\n")
	}
	return nil
}

func addComment(number, comment string) {
	cmt, err := m.AddComment(number, comment)
	if err != nil {
		gordon.Fatalf("%s", err)
	}
	gordon.DisplayCommentAdded(cmt)
}

func mainCmd(c *cli.Context) error {
	if !c.Args().Present() {
		var issues, err = m.GetIssues("open", c.String("assigned"))

		if err != nil {
			gordon.Fatalf("Error getting issues: %s", err)
		}
		issues, err = filters.FilterIssues(c, issues)
		if err != nil {
			gordon.Fatalf("Error filtering issues: %s", err)
		}

		fmt.Printf("%c[2K\r", 27)
		gordon.DisplayIssues(c, issues, c.Bool("no-trunc"))
		return nil
	}

	var (
		number  = c.Args().Get(0)
		comment = c.String("comment")
	)

	if comment != "" {
		addComment(number, comment)
		return nil
	}

	if c.Bool("vote") {
		addComment(number, "+1")
		fmt.Printf("Vote added to the issue: %s", number)
		return nil
	}

	issue, comments, err := m.GetIssue(number, true)
	if err != nil {
		gordon.Fatalf("%s", err)
	}
	gordon.DisplayIssue(issue, comments)
	return nil
}

func authCmd(c *cli.Context) error {
	config, err := gordon.LoadConfig()
	if err != nil {
		config = &gordon.Config{}
	}
	token := c.String("add")
	userName := c.String("user")
	if userName != "" {
		config.UserName = userName
		if err := gordon.SaveConfig(*config); err != nil {
			gordon.Fatalf("%s", err)
		}
	}
	if token != "" {
		config.Token = token
		if err := gordon.SaveConfig(*config); err != nil {
			gordon.Fatalf("%s", err)
		}
	}
	// Display token and user information
	if config, err := gordon.LoadConfig(); err == nil {
		if config.UserName != "" {
			fmt.Printf("Token: %s, UserName: %s\n", config.Token, config.UserName)
		} else {

			fmt.Printf("Token: %s\n", config.Token)
		}
	} else {
		fmt.Fprintf(os.Stderr, "No token registered\n")
		os.Exit(1)
	}
	return nil
}

func before(c *cli.Context) error {
	client := gh.NewClient()

	// set up the git remote to be used
	org, name, err := gordon.GetRemoteUrl(c.String("remote"))
	if err != nil {
		return fmt.Errorf("The current directory is not a valid git repository (%s).\n", err)
	}
	t, err := gordon.NewMaintainerManager(client, org, name)
	if err != nil {
		return err
	}
	m = t

	// Set verbosity
	gordon.VerboseOutput = c.Bool("verbose")

	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "issues"
	app.Usage = "Manage github issues"
	app.Version = gordon.Version

	app.Before = before
	loadCommands(app)

	err := app.Run(os.Args)
	if err != nil {
		gordon.Fatalf(err.Error())
	}
}
