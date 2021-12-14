package ssmconfig

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/stretchr/testify/assert"
)

type mockSSM struct {
	ssmiface.SSMAPI
	paramsOutput *ssm.GetParametersOutput
	paramsInput  *ssm.GetParametersInput
	paramsErr    error
}

func (s *mockSSM) GetParameters(input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	s.paramsInput = input
	return s.paramsOutput, s.paramsErr
}

type NestedStringConfig struct {
	NestedKey string `ssmparam:"/nested/key"`
}
type Config struct {
	TestKey        string            `ssmparam:"/test/key"`
	TestNumber     int               `ssmparam:"/test/number"`
	TestUInt       uint              `ssmparam:"/test/uint"`
	TestBool       bool              `ssmparam:"/test/bool"`
	TestFloat      float64           `ssmparam:"/test/float"`
	TestFloatSlice []float64         `ssmparam:"/test/floatslice"`
	TestDuration   time.Duration     `ssmparam:"/test/duration"`
	TestMap        map[string]string `ssmparam:"/test/map"`
	Nested         NestedStringConfig
	Inline         struct {
		InlineKey string `ssmparam:"/inline/key"`
	}
}

type NoTagConfig struct {
	TestKey string
}

type SmallConfig struct {
	Nested NestedStringConfig `ssmparam:"/prefixnested"`
}

func TestProcessCanParse(suite *testing.T) {
	tests := []struct {
		name               string
		ssmsvc             *mockSSM
		prefix             string
		config             interface{}
		expectedConfig     interface{}
		expectedErr        error
		expectedSSMRequest *ssm.GetParametersInput
	}{
		{
			name: "reads values from ssm",
			ssmsvc: &mockSSM{
				paramsOutput: &ssm.GetParametersOutput{
					Parameters: []*ssm.Parameter{
						{
							Name:  aws.String("/test/key"),
							Value: aws.String("testkey"),
						},
						{
							Name:  aws.String("/test/number"),
							Value: aws.String("-314"),
						},
						{
							Name:  aws.String("/test/uint"),
							Value: aws.String("314"),
						},
						{
							Name:  aws.String("/test/bool"),
							Value: aws.String("true"),
						},
						{
							Name:  aws.String("/test/float"),
							Value: aws.String("3.14159"),
						},
						{
							Name:  aws.String("/test/floatslice"),
							Value: aws.String("3.14159,2.618034,2.718"),
						},
						{
							Name:  aws.String("/test/duration"),
							Value: aws.String("5m"),
						},
						{
							Name:  aws.String("/test/map"),
							Value: aws.String(`first_name:Test,last_name:McTest,email:Test@test.com`),
						},
						{
							Name:  aws.String("/nested/key"),
							Value: aws.String("nestedkey"),
						},
						{
							Name:  aws.String("/inline/key"),
							Value: aws.String("inlinekey"),
						},
					},
				},
			},
			prefix: "",
			config: &Config{},
			expectedConfig: &Config{
				Nested: NestedStringConfig{
					NestedKey: "nestedkey",
				},
				TestKey:        "testkey",
				TestNumber:     -314,
				TestUInt:       314,
				TestBool:       true,
				TestFloat:      3.14159,
				TestFloatSlice: []float64{3.14159, 2.618034, 2.718},
				TestDuration:   5 * time.Minute,
				TestMap: map[string]string{
					"first_name": "Test",
					"last_name":  "McTest",
					"email":      "Test@test.com",
				},
				Inline: struct {
					InlineKey string `ssmparam:"/inline/key"`
				}{
					InlineKey: "inlinekey",
				},
			},
			expectedSSMRequest: &ssm.GetParametersInput{
				Names: []*string{
					aws.String("/test/key"),
					aws.String("/test/number"),
					aws.String("/test/uint"),
					aws.String("/test/bool"),
					aws.String("/test/float"),
					aws.String("/test/floatslice"),
					aws.String("/test/duration"),
					aws.String("/test/map"),
					aws.String("/nested/key"),
					aws.String("/inline/key"),
				},
				WithDecryption: aws.Bool(true),
			},
		},
		{
			name: "ignores values that are not marked",
			ssmsvc: &mockSSM{
				paramsOutput: &ssm.GetParametersOutput{
					Parameters: []*ssm.Parameter{
						{
							Name:  aws.String("/test/key"),
							Value: aws.String("testkey"),
						},
					},
				},
			},
			prefix: "",
			config: &NoTagConfig{
				TestKey: "nooverride",
			},
			expectedConfig: &NoTagConfig{
				TestKey: "nooverride",
			},
			expectedSSMRequest: &ssm.GetParametersInput{
				Names:          []*string{},
				WithDecryption: aws.Bool(true),
			},
		},
		{
			name: "prefixs should build as process traverses config object",
			ssmsvc: &mockSSM{
				paramsOutput: &ssm.GetParametersOutput{
					Parameters: []*ssm.Parameter{
						{
							Name:  aws.String("/test/prefixnested/nested/key"),
							Value: aws.String("nestedkey"),
						},
					},
				},
			},
			prefix: "/test",
			config: &SmallConfig{},
			expectedConfig: &SmallConfig{
				Nested: NestedStringConfig{
					NestedKey: "nestedkey",
				},
			},
			expectedSSMRequest: &ssm.GetParametersInput{
				Names: []*string{
					aws.String("/test/prefixnested/nested/key"),
				},
				WithDecryption: aws.Bool(true),
			},
		},
		{
			name:        "errs if bad param config is nil",
			prefix:      "",
			config:      nil,
			expectedErr: errors.New("spec must be non-nil pointer"),
		},
		{
			name:        "errs if bad param config is not ptr",
			prefix:      "",
			config:      1,
			expectedErr: errors.New("spec must be non-nil pointer"),
		},
		{
			name:        "errs if bad param config not struct",
			prefix:      "",
			config:      aws.Int(1),
			expectedErr: errors.New("spec must be a struct type"),
		},
		{
			name: "errs if fails to fetch from ssm",
			ssmsvc: &mockSSM{
				paramsErr: errors.New("test"),
			},
			prefix:      "",
			config:      &Config{},
			expectedErr: errors.New("test"),
		},
	}
	for _, test := range tests {
		suite.Run(test.name, func(t *testing.T) {
			err := Process(test.ssmsvc, test.prefix, test.config)
			assert.Equal(t, test.expectedErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, test.expectedConfig, test.config)
			assert.ElementsMatch(t, test.expectedSSMRequest.Names, test.ssmsvc.paramsInput.Names)
		})
	}
}
