package discoverers

import (
	"fmt"
	"strings"

	"github.com/abesto/easyssh/target"
	"github.com/abesto/easyssh/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type awsEc2TagSearch struct {
	awsRegion  string
	awsFilters []ec2.Filter
}

type awsEc2TagPostFilter struct {
	Key   string
	Value string
}

func (s *awsEc2TagSearch) Discover(input string) []target.Target {
	var targets []target.Target

	if !strings.Contains(input, "=") {
		input = fmt.Sprintf("Name={%s}", input)
	}

	session, err := session.NewSession()
	if err != nil {
		util.Panicf("Unable to create AWS session: %s", err)
	}

	svc := ec2.New(session, &aws.Config{Region: &s.awsRegion})

	ec2filters, postFilters := s.compileFilters(input)
	params := &ec2.DescribeInstancesInput{DryRun: aws.Bool(false),
		Filters: ec2filters}

	resp, err := svc.DescribeInstances(params)
	if err != nil {
		util.Panicf("Couldn't get matching instance from AWS: %s", err)
	}

	for _, reservation := range resp.Reservations {
		for _, inst := range reservation.Instances {
			if s.matchesToFilters(inst, postFilters) {
				tgt := target.Target{Host: *inst.InstanceId, IP: *inst.PublicIpAddress}
				targets = append(targets, tgt)
			}
		}
	}

	return targets
}

func (s *awsEc2TagSearch) compileFilters(input string) ([]*ec2.Filter, []*awsEc2TagPostFilter) {
	var ec2filters []*ec2.Filter
	var postFilters []*awsEc2TagPostFilter

	keyFilter := ec2.Filter{
		Name:   aws.String("tag-key"),
		Values: []*string{},
	}

	valueFilter := ec2.Filter{
		Name:   aws.String("tag-value"),
		Values: []*string{},
	}

	for _, clause := range strings.Split(input, ":") {
		inputParts := strings.SplitN(clause, "=", 2)

		if len(input) < 2 {
			util.Panicf("Filter must be in the format of TagKey=TagValue")
		}

		keyFilter.Values = append(keyFilter.Values, aws.String(inputParts[0]))
		valueFilter.Values = append(valueFilter.Values, aws.String(inputParts[1]))

		postFilters = append(postFilters, &awsEc2TagPostFilter{Key: inputParts[0], Value: inputParts[1]})
	}

	ec2filters = append(ec2filters, &keyFilter)
	ec2filters = append(ec2filters, &valueFilter)

	return ec2filters, postFilters
}

func (s *awsEc2TagSearch) matchesToFilters(instance *ec2.Instance, filters []*awsEc2TagPostFilter) bool {
	var numberOfTagsFound = 0

	for _, tag := range instance.Tags {
		for _, filter := range filters {
			if *tag.Key == filter.Key {
				numberOfTagsFound++

				if *tag.Value != filter.Value {
					return false
				}
			}
		}
	}

	return numberOfTagsFound == len(filters)
}

func (s *awsEc2TagSearch) SetArgs(args []interface{}) {
	util.RequireArguments(s, 1, args)
	util.RequireOnPath(s, "aws")
	s.awsRegion = string(args[0].([]uint8))
}

func (s *awsEc2TagSearch) String() string {
	return fmt.Sprintf("<%s>", nameAwsEc2Tag)
}
