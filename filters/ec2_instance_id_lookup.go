package filters

import (
	"encoding/json"
	"fmt"
	"github.com/abesto/easyssh/target"
	"github.com/abesto/easyssh/util"
	"regexp"
	"strings"
)

var ec2InstanceIdRegex = regexp.MustCompile("i-[0-9a-f]{8}")

type ec2InstanceIdParser interface {
	Parse(input string) string
}
type realEc2InstanceIdParser struct{}

func (p realEc2InstanceIdParser) Parse(input string) string {
	return ec2InstanceIdRegex.FindString(input)
}

type ec2InstanceIdLookup struct {
	region        string
	commandRunner util.CommandRunner
	idParser      ec2InstanceIdParser
}

func (f *ec2InstanceIdLookup) Filter(targets []target.Target) []target.Target {
	if f.region == "" {
		panic(fmt.Sprintf("%s requires exactly one argument, the region name to use for looking up instances", nameEc2InstanceId))
	}
	// TODO: build map by parsed id, do a single query using --instance-ids
	for idx, t := range targets {
		instanceId := f.idParser.Parse(t.Host)
		if len(instanceId) > 0 {
			util.Logger.Infof("EC2 Instance lookup: %s in %s", instanceId, f.region)

			output, err := f.commandRunner.RunGetOutput("aws", []string{"ec2", "describe-instances", "--instance-id", instanceId, "--region", f.region})
			util.Logger.Debugf("Response from AWS API: %s", output)
			if err != nil {
				util.Logger.Infof("EC2 Instance lookup failed for %s (%s) in region %s (aws command failed): %s", t.Host, instanceId, f.region, strings.TrimSpace(string(output)))
				continue
			}

			var data map[string]interface{}
			if err = json.Unmarshal(output, &data); err != nil {
				panic(fmt.Sprintf("Invalid JSON returned by AWS API.\nError: %s\nJSON follows this line\n%s", err.Error(), output))
			}

			var reservations = data["Reservations"]
			if reservations == nil || len(reservations.([]interface{})) == 0 {
				util.Logger.Infof("EC2 instance lookup failed for %s (%s) in region %s (Reservations is empty in the received JSON)", t.Host, instanceId, f.region)
				continue
			}
			targets[idx].Host = reservations.([]interface{})[0].(map[string]interface{})["Instances"].([]interface{})[0].(map[string]interface{})["PublicIpAddress"].(string)
		} else {
			util.Logger.Debugf("Target %s looks like it doesn't have EC2 instance ID, skipping lookup for region %s", t, f.region)
		}
	}
	return targets
}
func (f *ec2InstanceIdLookup) SetArgs(args []interface{}) {
	if len(args) != 1 {
		panic(fmt.Sprintf("%s requires exactly one argument, the region name to use for looking up instances", nameEc2InstanceId))
	}
	f.region = string(args[0].([]byte))
}
func (f *ec2InstanceIdLookup) String() string {
	return fmt.Sprintf("<%s %s>", nameEc2InstanceId, f.region)
}