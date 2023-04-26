package itemadd

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdaddItem(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       addItemOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "missing-url",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"url\" not set",
		},
		{
			name:        "user-and-org",
			cli:         "--user monalisa --org github --url github.com/cli/cli",
			wantsErr:    true,
			wantsErrMsg: "if any flags in the group [user org] are set none of the others can be; [org user] were all set",
		},
		{
			name:        "not-a-number",
			cli:         "x --url github.com/cli/cli",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "url",
			cli:  "--url github.com/cli/cli",
			wants: addItemOpts{
				itemURL: "github.com/cli/cli",
			},
		},
		{
			name: "number",
			cli:  "123 --url github.com/cli/cli",
			wants: addItemOpts{
				number:  123,
				itemURL: "github.com/cli/cli",
			},
		},
		{
			name: "user",
			cli:  "--user monalisa --url github.com/cli/cli",
			wants: addItemOpts{
				userOwner: "monalisa",
				itemURL:   "github.com/cli/cli",
			},
		},
		{
			name: "org",
			cli:  "--org github --url github.com/cli/cli",
			wants: addItemOpts{
				orgOwner: "github",
				itemURL:  "github.com/cli/cli",
			},
		},
		{
			name: "json",
			cli:  "--format json --url github.com/cli/cli",
			wants: addItemOpts{
				format:  "json",
				itemURL: "github.com/cli/cli",
			},
		},
	}

	os.Setenv("GH_TOKEN", "auth-token")
	defer os.Unsetenv("GH_TOKEN")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts addItemOpts
			cmd := NewCmdAddItem(f, func(config addItemConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.userOwner, gotOpts.userOwner)
			assert.Equal(t, tt.wants.orgOwner, gotOpts.orgOwner)
			assert.Equal(t, tt.wants.itemURL, gotOpts.itemURL)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunAddItem_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserLogin.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
		})

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// get item ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query GetIssueOrPullRequest.*",
			"variables": map[string]interface{}{
				"url": "https://github.com/cli/go-gh/issues/1",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"resource": map[string]interface{}{
					"id":         "item ID",
					"__typename": "Issue",
				},
			},
		})

	// create item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation AddItem.*","variables":{"input":{"projectId":"an ID","contentId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "item ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := addItemConfig{
		tp: tableprinter.New(ios),
		opts: addItemOpts{
			userOwner: "monalisa",
			number:    1,
			itemURL:   "https://github.com/cli/go-gh/issues/1",
		},
		client: client,
	}

	err = runAddItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Added item\n",
		stdout.String())
}

func TestRunAddItem_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgLogin.*",
			"variables": map[string]interface{}{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id": "an ID",
				},
			},
		})

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgProject.*",
			"variables": map[string]interface{}{
				"login":       "github",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// get item ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query GetIssueOrPullRequest.*",
			"variables": map[string]interface{}{
				"url": "https://github.com/cli/go-gh/issues/1",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"resource": map[string]interface{}{
					"id":         "item ID",
					"__typename": "Issue",
				},
			},
		})

	// create item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation AddItem.*","variables":{"input":{"projectId":"an ID","contentId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "item ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := addItemConfig{
		tp: tableprinter.New(ios),
		opts: addItemOpts{
			orgOwner: "github",
			number:   1,
			itemURL:  "https://github.com/cli/go-gh/issues/1",
		},
		client: client,
	}

	err = runAddItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Added item\n",
		stdout.String())
}

func TestRunAddItem_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerLogin.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"id": "an ID",
				},
			},
		})

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerProject.*",
			"variables": map[string]interface{}{
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// get item ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query GetIssueOrPullRequest.*",
			"variables": map[string]interface{}{
				"url": "https://github.com/cli/go-gh/pull/1",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"resource": map[string]interface{}{
					"id":         "item ID",
					"__typename": "PullRequest",
				},
			},
		})

	// create item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation AddItem.*","variables":{"input":{"projectId":"an ID","contentId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"addProjectV2ItemById": map[string]interface{}{
					"item": map[string]interface{}{
						"id": "item ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := addItemConfig{
		tp: tableprinter.New(ios),
		opts: addItemOpts{
			userOwner: "@me",
			number:    1,
			itemURL:   "https://github.com/cli/go-gh/pull/1",
		},
		client: client,
	}

	err = runAddItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Added item\n",
		stdout.String())
}
