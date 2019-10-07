package command

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
	"github.com/github/gh-cli/ui"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prListCmd)
	prCmd.AddCommand(prShowCmd)
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prCheckoutCmd)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `Interact with pull requests for this repository.
`,
	Run: func(cmd *cobra.Command, args []string) {
		err := interactiveList()
		utils.Check(err)
	},
}

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List open pull requests related to you",
	Run: func(cmd *cobra.Command, args []string) {
		list()
	},
}

var prShowCmd = &cobra.Command{
	Use:   "show [<pr-number>]",
	Short: "Open a pull request in the browser",
	Long: `Opens the pull request in the web browser.

When <pr-number> is not given, the pull request that belongs to the current
branch is opened.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return show(args...)
	},
}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request",
	Run: func(cmd *cobra.Command, args []string) {
		createPr(args...)
	},
}

var prCheckoutCmd = &cobra.Command{
	Use:   "checkout <pr-number>",
	Short: "Check out a pull request in git",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return checkoutPr(args[0])
	},
}

type prFilter int

type prCreateInput struct {
	Title string
	Body  string
}

const (
	createdByViewer prFilter = iota
	reviewRequested
)

func determineEditor() string {
	// TODO THIS IS PROBABLY GROSS
	// I copied this from survey because i wanted to use the same logic as them
	// for now.
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	} else if e := os.Getenv("EDITOR"); e != "" {
		return e
	}

	return "nano"
}

func createPrSurvey(inProgress prCreateInput) (prCreateInput, error) {
	editor := determineEditor()
	qs := []*survey.Question{
		{
			Name: "title",
			Prompt: &survey.Input{
				Message: "PR Title",
				Default: inProgress.Title,
			},
		},
		{
			Name: "body",
			Prompt: &survey.Editor{
				Message:       fmt.Sprintf("PR Body (%s)", editor),
				FileName:      "*.md",
				Default:       inProgress.Body,
				AppendDefault: true,
				Editor:        editor,
			},
		},
	}

	err := survey.Ask(qs, &inProgress)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return inProgress, err
	}

	return inProgress, nil
}

func createPr(...string) {
	// TODO i wanted some information here:
	// - whether current branch was pushed yet
	// - whether PR was already open for this branch
	// - if working directory was dirty
	// - what branch is targeted
	// and also wanted to take some flags:
	// - --target for a different place to land a PR
	// - --no-push to disable the initial push

	confirmed := false

	inProgress := prCreateInput{}
	for !confirmed {
		inProgress, _ = createPrSurvey(inProgress)

		ui.Println(inProgress.Body)

		confirmAnswers := struct {
			Confirmation string
		}{}

		confirmQs := []*survey.Question{
			{
				Name: "confirmation",
				Prompt: &survey.Select{
					Message: "Submit?",
					Options: []string{
						"Yes",
						"Edit",
						"Cancel and discard",
					},
				},
			},
		}

		err := survey.Ask(confirmQs, &confirmAnswers)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			return
		}

		switch confirmAnswers.Confirmation {
		case "Yes":
			confirmed = true
		case "Edit":
			continue
		case "Cancel and discard":
			ui.Println("Discarding PR.")
			return
		}
	}

	// TODO this is quite silly for now; but i expect that survey will intake
	// slightly different data than the CPR API wants sooner than later
	prParams := github.PullRequestParams{
		Title: inProgress.Title,
		Body:  inProgress.Body,
	}

	err := github.CreatePullRequest(prParams)
	if err != nil {
		// I'd like this to go even higher but this works for now
		utils.Check(err)
	}
}

func interactiveList() error {
	currentPr, viewerCreated, reviewRequested, err := pullRequests()
	if err != nil {
		return err
	}

	prs := []graphqlPullRequest{}
	if currentPr != nil {
		prs = append(prs, *currentPr)
	}
	prs = append(prs, viewerCreated...)
	prs = append(prs, reviewRequested...)

	const openAction = "open in browser"
	const checkoutAction = "checkout PR locally"
	const cancelAction = "cancel"

	prOptions := []string{}
	seen := map[int]bool{}
	for _, pr := range prs {
		if seen[pr.Number] {
			continue
		}
		prOptions = append(prOptions, fmt.Sprintf("[%v] %s", pr.Number, pr.Title))
		seen[pr.Number] = true
	}

	// TODO figure out how to visually seperate the PR list
	qs := []*survey.Question{
		{
			Name: "pr",
			Prompt: &survey.Select{
				Message: "PRs you might be interested in",
				Options: prOptions,
			},
		},
		{
			Name: "action",
			Prompt: &survey.Select{
				Message: "What would you like to do?",
				Options: []string{
					openAction,
					checkoutAction,
					cancelAction,
				},
			},
		},
	}

	answers := struct {
		Pr     int
		Action string
	}{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	actions := map[string]func() error{}

	actions[cancelAction] = func() error { return nil }
	actions[openAction] = func() error {
		launcher, err := utils.BrowserLauncher()
		if err != nil {
			return err
		}
		exec.Command(launcher[0], prs[answers.Pr].URL).Run()
		return nil
	}
	actions[checkoutAction] = func() error {
		pr := prs[answers.Pr]
		return checkoutPr(fmt.Sprintf("%v", pr.Number))
	}

	return actions[answers.Action]()

}
func list() error {
	currentPr, viewerCreated, reviewRequested, err := pullRequests()
	if err != nil {
		return err
	}
	currentBranch := currentBranch()

	currentPrOutput := style(currentBranch, `{{- bold "Current branch "}}`) +
		style(currentPr, `
{{if .}}  #{{.Number}} {{.Title}} {{cyan "[" .HeadRefName "]"}}
{{else}}  {{gray "There is no pull request associated with this branch"}}
{{end}}`)

	viewerCreatedOutput := style(viewerCreated, `
{{bold "Pull requests created by you"}}
{{- if . }}
{{- range .}}
	#{{.Number}} {{.Title}} {{cyan "[" .HeadRefName "]"}}
{{- end}}
{{else}}
	{{gray "You have no pull requests open."}}
{{end}}`)

	reviewRequestedOutput := style(reviewRequested, `
{{bold "Pull requests requesting a code review from you"}}
{{- if . }}
{{- range .}}
	#{{.Number}} {{.Title}} {{cyan "[" .HeadRefName "]"}}
{{- end}}
{{else}}
	{{gray "You have no pull requests to review."}}
{{end}}`)

	fmt.Println(currentPrOutput + viewerCreatedOutput + reviewRequestedOutput)
	return nil
}

func show(number ...string) error {
	project := project()

	var openURL string
	if len(number) > 0 {
		if prNumber, err := strconv.Atoi(number[0]); err == nil {
			openURL = project.WebURL("", "", fmt.Sprintf("pull/%d", prNumber))
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", number[0])
		}
	} else {
		pr, err := pullRequestForCurrentBranch()
		if err != nil {
			return err
		}
		openURL = pr.HtmlUrl
	}

	return openInBrowser(openURL)
}

func pullRequestForCurrentBranch() (*github.PullRequest, error) {
	project := project()
	client := github.NewClient(project.Host)
	headWithOwner := fmt.Sprintf("%s:%s", project.Owner, currentBranch())

	filterParams := map[string]interface{}{"head": headWithOwner}
	prs, err := client.FetchPullRequests(&project, filterParams, 10, nil)
	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, fmt.Errorf("no pull requests found for the current branch")
	}

	return &prs[0], nil
}

// TODO: figure out a less ridiculous way to parse GraphQL response
type searchBody struct {
	Data searchData `json:"data"`
}
type searchData struct {
	Repository struct {
		PullRequests edges `json:"pullRequests"`
	} `json:"repository"`
	ViewerCreated   edges `json:"viewerCreated"`
	ReviewRequested edges `json:"reviewRequested"`
}
type edges struct {
	Edges    []nodes  `json:"edges"`
	PageInfo pageInfo `json:"pageInfo"`
}
type nodes struct {
	Node graphqlPullRequest `json:"node"`
}
type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Add entries here when requesting additional fields in the GraphQL query
type graphqlPullRequest struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
}

func pullRequests() (*graphqlPullRequest, []graphqlPullRequest, []graphqlPullRequest, error) {
	project := project()
	client := github.NewClient(project.Host)
	owner := project.Owner
	repo := project.Name
	currentBranch := currentBranch()

	var headers map[string]string
	viewerQuery := fmt.Sprintf("repo:%s/%s state:open is:pr author:%s", owner, repo, currentUsername())
	reviewerQuery := fmt.Sprintf("repo:%s/%s state:open review-requested:%s", owner, repo, currentUsername())

	variables := map[string]interface{}{
		"viewerQuery":   viewerQuery,
		"reviewerQuery": reviewerQuery,
		"owner":         owner,
		"repo":          repo,
		"headRefName":   currentBranch,
	}

	data := map[string]interface{}{
		"variables": variables,
		"query": `
fragment pr on PullRequest {
	number
	title
	url
	headRefName
}

query($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
	repository(owner: $owner, name: $repo) {
    pullRequests(headRefName: $headRefName, first: 1) {
			edges {
				node {
					...pr
				}
			}
		}
  }
	viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
		edges {
			node {
				...pr
			}
		}
		pageInfo {
			hasNextPage
		}
	}
	reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $per_page) {
		edges {
			node {
				...pr
			}
		}
		pageInfo {
			hasNextPage
		}
	}
}`}

	response, err := client.GenericAPIRequest("POST", "graphql", data, headers, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	responseBody := searchBody{}
	err = response.Unmarshal(&responseBody)
	if err != nil {
		return nil, nil, nil, err
	}

	viewerCreated := []graphqlPullRequest{}
	reviewRequested := []graphqlPullRequest{}
	for _, edge := range responseBody.Data.ViewerCreated.Edges {
		viewerCreated = append(viewerCreated, edge.Node)
	}
	for _, edge := range responseBody.Data.ReviewRequested.Edges {
		reviewRequested = append(reviewRequested, edge.Node)
	}
	var currentPr *graphqlPullRequest
	if len(responseBody.Data.Repository.PullRequests.Edges) > 0 {
		currentPr = &responseBody.Data.Repository.PullRequests.Edges[0].Node
	}
	return currentPr, viewerCreated, reviewRequested, nil
}

func currentBranch() string {
	currentBranch, err := git.Head()
	if err != nil {
		panic(err)
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1)
}

func project() github.Project {
	if repoFromEnv := os.Getenv("GH_REPO"); repoFromEnv != "" {
		repoURL, err := url.Parse(fmt.Sprintf("https://github.com/%s.git", repoFromEnv))
		if err != nil {
		}
		project, err := github.NewProjectFromURL(repoURL)
		if err != nil {
			panic(err)
		}
		return *project
	}

	remotes, err := github.Remotes()
	if err != nil {
		panic(err)
	}

	for _, remote := range remotes {
		if project, err := remote.Project(); err == nil {
			return *project
		}
	}

	panic("Could not get the project. What is a project? I don't know, it's kind of like a git repository I think?")
}

func openInBrowser(url string) error {
	launcher, err := utils.BrowserLauncher()
	if err != nil {
		return err
	}
	endingArgs := append(launcher[1:], url)
	return exec.Command(launcher[0], endingArgs...).Run()
}

func currentUsername() string {
	host, err := github.CurrentConfig().DefaultHost()
	if err != nil {
		panic(err)
	}
	return host.User
}

func checkoutPr(number string) error {
	_, err := strconv.Atoi(number)
	if err != nil {
		return err
	}

	project := project()
	client := github.NewClient(project.Host)
	pullRequest, err := client.PullRequest(&project, number)
	if err != nil {
		return err
	}

	repo, err := github.LocalRepo()
	if err != nil {
		return err
	}

	baseRemote, err := repo.RemoteForRepo(pullRequest.Base.Repo)
	if err != nil {
		return err
	}

	var headRemote *github.Remote
	if pullRequest.IsSameRepo() {
		headRemote = baseRemote
	} else if pullRequest.Head.Repo != nil {
		headRemote, _ = repo.RemoteForRepo(pullRequest.Head.Repo)
	}

	newBranchName := ""
	if headRemote != nil {
		if newBranchName == "" {
			newBranchName = pullRequest.Head.Ref
		}
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pullRequest.Head.Ref)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pullRequest.Head.Ref, remoteBranch)

		utils.Check(git.Run("fetch", headRemote.Name, refSpec))

		if git.HasFile("refs", "heads", newBranchName) {
			utils.Check(git.Run("checkout", newBranchName))
			utils.Check(git.Run("merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)))
		} else {
			utils.Check(git.Run("checkout", "-b", newBranchName, "--no-track", remoteBranch))
			utils.Check(git.Run("config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name))
			utils.Check(git.Run("config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/"+pullRequest.Head.Ref))
		}
	} else {
		if newBranchName == "" {
			newBranchName = pullRequest.Head.Ref
			if pullRequest.Head.Repo != nil && newBranchName == pullRequest.Head.Repo.DefaultBranch {
				newBranchName = fmt.Sprintf("%s-%s", pullRequest.Head.Repo.Owner.Login, newBranchName)
			}
		}

		ref := fmt.Sprintf("refs/pull/%d/head", pullRequest.Number)
		utils.Check(git.Run("fetch", baseRemote.Name, fmt.Sprintf("%s:%s", ref, newBranchName)))
		utils.Check(git.Run("checkout", newBranchName))

		remote := baseRemote.Name
		mergeRef := ref
		if pullRequest.MaintainerCanModify && pullRequest.Head.Repo != nil {
			headRepo := pullRequest.Head.Repo
			headProject := github.NewProject(headRepo.Owner.Login, headRepo.Name, project.Host)
			remote = headProject.GitURL("", "", true)
			mergeRef = fmt.Sprintf("refs/heads/%s", pullRequest.Head.Ref)
		}
		utils.Check(git.Run("config", fmt.Sprintf("branch.%s.remote", newBranchName), remote))
		utils.Check(git.Run("config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef))
	}
	return nil
}
