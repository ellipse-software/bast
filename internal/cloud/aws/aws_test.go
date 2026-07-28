package aws

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverMapsDirectAndEICEInstances(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "production.pem"), []byte("-----BEGIN PRIVATE KEY-----\nkey\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := New()
	client.Run = fakeAWS(t, func(joined string) (any, error) {
		switch {
		case strings.Contains(joined, "--version"):
			return "aws-cli/2.35.0", nil
		case strings.Contains(joined, "configure list-profiles"):
			return "default\n", nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return map[string]string{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}, nil
		case strings.Contains(joined, "describe-regions"):
			return map[string]any{"Regions": []map[string]string{{"RegionName": "eu-west-1"}}}, nil
		case strings.Contains(joined, "describe-instances"):
			return map[string]any{"Reservations": []any{map[string]any{"Instances": []any{
				map[string]any{"InstanceId": "i-public", "ImageId": "ami-ubuntu", "KeyName": "production", "PublicIpAddress": "203.0.113.10", "VpcId": "vpc-1", "SubnetId": "subnet-a", "Placement": map[string]string{"AvailabilityZone": "eu-west-1a"}, "Tags": []map[string]string{{"Key": "Name", "Value": "web"}}},
				map[string]any{"InstanceId": "i-private", "ImageId": "ami-amazon", "PrivateIpAddress": "10.0.0.2", "VpcId": "vpc-1", "SubnetId": "subnet-b", "Placement": map[string]string{"AvailabilityZone": "eu-west-1b"}},
				map[string]any{"InstanceId": "i-windows", "Platform": "windows", "ImageId": "ami-win", "PublicIpAddress": "203.0.113.11"},
			}}}}, nil
		case strings.Contains(joined, "describe-images"):
			return map[string]any{"Images": []any{
				map[string]string{"ImageId": "ami-ubuntu", "Name": "ubuntu/images/hvm-ssd"},
				map[string]string{"ImageId": "ami-amazon", "Name": "al2023-ami"},
			}}, nil
		case strings.Contains(joined, "describe-instance-connect-endpoints"):
			return map[string]any{"InstanceConnectEndpoints": []any{
				map[string]string{"InstanceConnectEndpointId": "eice-other", "State": "create-complete", "VpcId": "vpc-1", "SubnetId": "subnet-a"},
				map[string]string{"InstanceConnectEndpointId": "eice-same", "State": "create-complete", "VpcId": "vpc-1", "SubnetId": "subnet-b"},
			}}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	instances, err := client.Discover(context.Background(), DiscoverConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %+v", instances)
	}
	byName := map[string]Instance{}
	for _, inst := range instances {
		byName[inst.Name] = inst
	}
	web := byName["web"]
	if web.HostName != "203.0.113.10" || web.User != "ubuntu" || web.IdentityFile != "~/.ssh/production.pem" || web.UseEICE {
		t.Fatalf("direct instance = %+v", web)
	}
	private := byName["i-private"]
	if !private.UseEICE || private.EndpointID != "eice-same" || private.HostName != "i-private" || private.User != "ec2-user" {
		t.Fatalf("private instance = %+v", private)
	}
	block := ToSyncHost(private, "aws_default_eu-west-1_i-private")
	if !strings.Contains(block.ProxyCommand, "open-tunnel") || !strings.Contains(block.ProxyCommand, "eice-same") || !strings.Contains(block.ProxyCommand, "--remote-port=%p") || strings.Contains(block.ProxyCommand, "%%p") {
		t.Fatalf("proxy command = %q", block.ProxyCommand)
	}
}

func TestGroupPathUsesAmazonEC2Namespace(t *testing.T) {
	inst := Instance{Profile: "production", Region: "eu-west-2"}
	if got := GroupPath(inst); got != "Amazon EC2/production/eu-west-2" {
		t.Fatalf("GroupPath = %q", got)
	}
}

func TestDiscoverRejectsPartialInventory(t *testing.T) {
	client := New()
	client.Run = fakeAWS(t, func(joined string) (any, error) {
		switch {
		case strings.Contains(joined, "--version"):
			return "aws-cli/2", nil
		case strings.Contains(joined, "configure list-profiles"):
			return "default\n", nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return map[string]string{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}, nil
		case strings.Contains(joined, "describe-regions"):
			return map[string]any{"Regions": []map[string]string{{"RegionName": "eu-west-1"}, {"RegionName": "us-east-1"}}}, nil
		case strings.Contains(joined, "describe-instances") && strings.Contains(joined, "us-east-1"):
			return nil, errors.New("access denied")
		case strings.Contains(joined, "describe-instances"):
			return map[string]any{"Reservations": []any{}}, nil
		case strings.Contains(joined, "describe-instance-connect-endpoints"):
			return map[string]any{"InstanceConnectEndpoints": []any{}}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	instances, err := client.Discover(context.Background(), DiscoverConfig{})
	if err == nil || !strings.Contains(err.Error(), "incomplete AWS discovery") || instances != nil {
		t.Fatalf("instances=%+v err=%v", instances, err)
	}
}

func TestEnsureAccessUsesLaunchKeyWithoutPublishing(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "prod.pem"), []byte("-----BEGIN PRIVATE KEY-----\nkey\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := New()
	client.Run = fakeAWS(t, func(joined string) (any, error) {
		switch {
		case strings.Contains(joined, "--version"):
			return "aws-cli/2", nil
		case strings.Contains(joined, "configure list-profiles"):
			return "default\n", nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return map[string]string{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}, nil
		case strings.Contains(joined, "describe-instances"):
			return map[string]any{"Reservations": []any{map[string]any{"Instances": []any{map[string]any{
				"InstanceId": "i-123", "ImageId": "ami-1", "KeyName": "prod", "Placement": map[string]string{"AvailabilityZone": "eu-west-1a"},
			}}}}}, nil
		case strings.Contains(joined, "describe-images"):
			return map[string]any{"Images": []any{map[string]string{"ImageId": "ami-1", "Name": "ubuntu"}}}, nil
		case strings.Contains(joined, "send-ssh-public-key"):
			t.Fatal("launch key should avoid EC2 Instance Connect publication")
		}
		return nil, errors.New("unexpected command: " + joined)
	})
	result, err := client.EnsureAccess(context.Background(), "arn:aws:ec2:eu-west-1:123456789012:instance/i-123", EnsureConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if result.User != "ubuntu" || result.IdentityFile != "~/.ssh/prod.pem" || !result.IdentitiesOnly || result.KeyAdded {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureAccessPublishesManagedKey(t *testing.T) {
	home := t.TempDir()
	client := New()
	published := false
	client.Run = fakeAWS(t, func(joined string) (any, error) {
		switch {
		case strings.Contains(joined, "--version"):
			return "aws-cli/2", nil
		case strings.Contains(joined, "configure list-profiles"):
			return "default\n", nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return map[string]string{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}, nil
		case strings.Contains(joined, "describe-instances"):
			return map[string]any{"Reservations": []any{map[string]any{"Instances": []any{map[string]any{
				"InstanceId": "i-123", "Placement": map[string]string{"AvailabilityZone": "eu-west-1a"},
			}}}}}, nil
		case strings.Contains(joined, "send-ssh-public-key"):
			published = true
			if !strings.Contains(joined, "--instance-os-user ec2-user") || !strings.Contains(joined, "aws_compute.pub") {
				t.Fatalf("publish args = %s", joined)
			}
			return map[string]any{"Success": true}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	result, err := client.EnsureAccess(context.Background(), "arn:aws:ec2:eu-west-1:123456789012:instance/i-123", EnsureConfig{Home: home, DefaultSSHUser: "ec2-user"})
	if err != nil {
		t.Fatal(err)
	}
	if !published || !result.KeyAdded || result.IdentityFile != awsIdentityFile {
		t.Fatalf("published=%t result=%+v", published, result)
	}
	for _, path := range []string{filepath.Join(home, ".ssh", "bast", "aws_compute"), filepath.Join(home, ".ssh", "bast", "aws_compute.pub")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated key %s: %v", path, err)
		}
	}
}

func TestEnsureAccessReportsProfileAuthenticationAction(t *testing.T) {
	client := New()
	client.Run = fakeAWS(t, func(joined string) (any, error) {
		switch {
		case strings.Contains(joined, "--version"):
			return "aws-cli/2", nil
		case strings.Contains(joined, "configure list-profiles"):
			return "test\n", nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return nil, errors.New("SSO session expired")
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	_, err := client.EnsureAccess(
		context.Background(),
		"arn:aws:ec2:eu-west-1:123456789012:instance/i-123",
		EnsureConfig{},
	)
	if err == nil || !strings.Contains(err.Error(), `authenticate AWS profile "test"`) {
		t.Fatalf("authentication error = %v", err)
	}
	if !strings.Contains(err.Error(), `aws sso login --profile "test"`) || !strings.Contains(err.Error(), `aws configure --profile "test"`) {
		t.Fatalf("authentication error is not actionable: %v", err)
	}
}

func TestParseSyncIDAndProfileOrder(t *testing.T) {
	partition, region, account, instance, err := ParseSyncID("arn:aws-us-gov:ec2:us-gov-west-1:123456789012:instance/i-123")
	if err != nil || partition != "aws-us-gov" || region != "us-gov-west-1" || account != "123456789012" || instance != "i-123" {
		t.Fatalf("parsed = %q %q %q %q err=%v", partition, region, account, instance, err)
	}
	got := orderProfiles([]string{"zeta", "default", "active"}, "active")
	if strings.Join(got, ",") != "active,default,zeta" {
		t.Fatalf("profiles = %v", got)
	}
}

func fakeAWS(t *testing.T, response func(joined string) (any, error)) Runner {
	t.Helper()
	return func(ctx context.Context, args []string, env []string) ([]byte, error) {
		value, err := response(strings.Join(args[1:], " "))
		if err != nil {
			return nil, err
		}
		if text, ok := value.(string); ok {
			return []byte(text), nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data, nil
	}
}
