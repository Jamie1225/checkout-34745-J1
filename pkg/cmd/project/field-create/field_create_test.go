package fieldcreate

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

func TestNewCmdCreateField(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       createFieldOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "missing-name-and-data-type",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"data-type\", \"name\" not set",
		},
		{
			name:        "user-and-org",
			cli:         "--user monalisa --org github  --name n --data-type TEXT",
			wantsErr:    true,
			wantsErrMsg: "if any flags in the group [user org] are set none of the others can be; [org user] were all set",
		},
		{
			name:        "not-a-number",
			cli:         "x  --name n --data-type TEXT",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name:        "single-select-no-options",
			cli:         "123 --name n --data-type SINGLE_SELECT",
			wantsErr:    true,
			wantsErrMsg: "at least one single select options is required with data type is SINGLE_SELECT",
		},
		{
			name: "number",
			cli:  "123 --name n --data-type TEXT",
			wants: createFieldOpts{
				number:              123,
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "user",
			cli:  "--user monalisa --name n --data-type TEXT",
			wants: createFieldOpts{
				userOwner:           "monalisa",
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "org",
			cli:  "--org github --name n --data-type TEXT",
			wants: createFieldOpts{
				orgOwner:            "github",
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "single-select-options",
			cli:  "--name n --data-type TEXT --single-select-options a,b",
			wants: createFieldOpts{
				singleSelectOptions: []string{"a", "b"},
				name:                "n",
				dataType:            "TEXT",
			},
		},
		{
			name: "json",
			cli:  "--format json --name n --data-type TEXT ",
			wants: createFieldOpts{
				format:              "json",
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
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

			var gotOpts createFieldOpts
			cmd := NewCmdCreateField(f, func(config createFieldConfig) error {
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
			assert.Equal(t, tt.wants.name, gotOpts.name)
			assert.Equal(t, tt.wants.dataType, gotOpts.dataType)
			assert.Equal(t, tt.wants.singleSelectOptions, gotOpts.singleSelectOptions)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunCreateField_User(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			name:      "a name",
			userOwner: "monalisa",
			number:    1,
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_Org(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			name:     "a name",
			orgOwner: "github",
			number:   1,
			dataType: "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_Me(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_TEXT(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_DATE(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"DATE","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "DATE",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_NUMBER(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"NUMBER","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		tp: tableprinter.New(ios),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "NUMBER",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}
