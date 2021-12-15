# ssmconfig
Gets Configuration values from AWS stored SSM and meant to integrate with other config frameworks which may not features which support ssm params.

## Example

```
package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/kelseyhightower/envconfig"
	"github.com/tquocpham/ssmconfig"
)

type Config struct {
	LogLevel        string `envconfig:"LOG_LEVEL"   default:"info"`
	WebPort         string `envconfig:"WEB_PORT"    default:"8000"`
	Environment     string `envconfig:"ENVIRONMENT" default:"local"`
	SomeKey string `envconfig:"STORE_LICENSE_KEY"   default:"000000" ssmparam:"/some/licensekey"`
}

func main() {
	var config Config
	if err := envconfig.Process("", &config); err != nil {
		panic(err)
	}

	if config.Environment != "local" {
		sess, err := session.NewSession()
		if err != nil {
			panic(err)
		}
		ssmsvc := ssm.New(sess)
		if err := ssmconfig.Process(ssmsvc, "/prefix", &config); err != nil {
			panic(err)
		}
	}
}
```
