package ams

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type Source struct {
	secretName string
	region     string
}

type Option func(*Source)

func (s *Source) Read() ([]byte, error) {
	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(s.region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	// Create SSM client
	svc := ssm.New(sess)

	// Get the parameter value
	param, err := svc.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(s.secretName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter %s: %w", s.secretName, err)
	}

	if param.Parameter == nil || param.Parameter.Value == nil {
		return nil, fmt.Errorf("parameter %s has no value", s.secretName)
	}

	return []byte(*param.Parameter.Value), nil
}

func NewSource(opts ...Option) *Source {
	s := &Source{
		region: "eu-central-1", // default region
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithSecretName(name string) Option {
	fmt.Printf("Secret name: %s\n", name)
	return func(o *Source) {
		o.secretName = name
	}
}

func WithRegion(region string) Option {
	return func(o *Source) {
		o.region = region
	}
}
