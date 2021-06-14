package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"bufio"
	"encoding/base64"
	"io/ioutil"
	"regexp"
)

var AWSImageListLock *sync.Mutex = &sync.Mutex{}
var AWSImageList string

func AWSGetImageID(region string) (string, error) {
	AWSImageListLock.Lock()
	if AWSImageList == "" {
		resp, err := http.Get("https://cloud-images.ubuntu.com/locator/ec2/releasesTable")
		if err != nil {
			return "", err
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		AWSImageList = string(body)
		defer resp.Body.Close()
	}
	AWSImageListLock.Unlock()

	escaped := strings.ReplaceAll(region, "-", `\-`)
	tr := escaped + `","focal","20\.04 LTS","amd64",".+","\d+","<.+>(ami\-.+)</a>`
	r := regexp.MustCompile(tr)

	s := bufio.NewScanner(strings.NewReader(AWSImageList))
	s.Buffer(nil, 8*1024*1024)
	var res string
	for s.Scan() {
		l := s.Text()
		m := r.FindStringSubmatch(l)
		if len(m) != 0 {
			res = m[1]
			break
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return res, nil
}

type EC2Region struct {
	Endpoint string
	Name     string
}

func AWSListRegions(sess *session.Session) ([]EC2Region, error) {
	svc := ec2.New(sess)

	input := &ec2.DescribeRegionsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("opt-in-status"),
				Values: []*string{aws.String("opt-in-not-required"), aws.String("opted-in")},
			},
		},
	}

	res, err := svc.DescribeRegions(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, aerr
		} else {
			return nil, err
		}
	}

	output := []EC2Region{}

	for _, r := range res.Regions {
		output = append(output, EC2Region{*r.Endpoint, *r.RegionName})
	}

	return output, nil
}

func AWSGetSecurityGroup(sess *session.Session) (string, error) {
	svc := ec2.New(sess)

	input := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String("pika")},
			},
		},
	}
	var groups []string
	handle := func(page *ec2.DescribeSecurityGroupsOutput, last bool) bool {
		for _, g := range page.SecurityGroups {
			groups = append(groups, *g.GroupId)
		}
		return true
	}
	err := svc.DescribeSecurityGroupsPages(input, handle)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}

	if len(groups) == 0 {
		return "", nil
	} else if len(groups) > 1 {
		panic("multiple keypairs")
	} else {
		return groups[0], nil
	}
}

func AWSGetVPC(sess *session.Session) (string, error) {
	// Get a list of VPCs so we can associate the group with the first VPC.
	svc := ec2.New(sess)
	result, err := svc.DescribeVpcs(nil)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}
	if len(result.Vpcs) == 0 {
		panic("no VPC found")
	}
	return *result.Vpcs[0].VpcId, nil
}

func AWSCreateSecurityGroup(sess *session.Session) (string, error) {
	vpcid, err := AWSGetVPC(sess)
	if err != nil {
		return "", err
	}
	svc := ec2.New(sess)
	input := &ec2.CreateSecurityGroupInput{
		Description: aws.String("allow all in all out"),
		GroupName:   aws.String("pika-allow-all"),
		VpcId:       aws.String(vpcid),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("security-group"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("pika"),
						Value: aws.String("security-group"),
					},
				},
			},
		},
	}

	createres, err := svc.CreateSecurityGroup(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}
	output := *createres.GroupId

	// add permissions
	_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(output),
		IpPermissions: []*ec2.IpPermission{
			// Can use setters to simplify seting multiple values without the
			// needing to use aws.String or associated helper utilities.
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				}),
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}
	/*
		// it seems that egress rule is already there
		_, err = svc.AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
			GroupId: aws.String(output),
			IpPermissions: []*ec2.IpPermission{
				// Can use setters to simplify seting multiple values without the
				// needing to use aws.String or associated helper utilities.
				(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				}),
			},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				return "", aerr
			} else {
				return "", err
			}
		}
	*/

	return output, nil
}

func AWSGetSSHKey(sess *session.Session) (string, error) {
	svc := ec2.New(sess)

	input := &ec2.DescribeKeyPairsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String("pika")},
			},
		},
	}
	result, err := svc.DescribeKeyPairs(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}

	if len(result.KeyPairs) == 0 {
		return "", nil
	} else if len(result.KeyPairs) > 1 {
		panic("multiple keypairs")
	} else {
		return *result.KeyPairs[0].KeyName, nil
	}
}

func AWSUploadSSHKey(sess *session.Session, source string) (string, error) {
	svc := ec2.New(sess)

	pk, err := getPublicKey(source)
	if err != nil {
		return "", err
	}

	result, err := svc.ImportKeyPair(&ec2.ImportKeyPairInput{
		KeyName: aws.String("pikabarrow"),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("key-pair"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("pika"),
						Value: aws.String("ssh-key"),
					},
				},
			},
		},
		PublicKeyMaterial: pk,
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return "", aerr
		} else {
			return "", err
		}
	}

	return *result.KeyName, nil
}

func AWSStartInstanceAtRegions(region string, count int, tag string) error {
	regions, err := AWSFilterRegionsByName(region)
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	for _, r := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			err := AWSStartInstanceAtRegion(r, count, tag)
			if err != nil {
				fmt.Println(err)
			}
		}(r)
	}
	wg.Wait()
	return nil
}

func AWSStartInstanceAtRegion(region string, count int, tag string) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "pika"),
	})

	// get the AMI ID for ubuntu 20.04
	ami, err := AWSGetImageID(region)
	if err != nil {
		return err
	}

	// get the ssh key
	keyid, err := AWSGetSSHKey(sess)
	if err != nil {
		return err
	}
	if keyid == "" {
		// upload the ssh key
		keyid, err = AWSUploadSSHKey(sess, "/Users/leiy/.ssh/pikaaws")
		if err != nil {
			return err
		}
	}

	// get the security group id
	sgid, err := AWSGetSecurityGroup(sess)
	if err != nil {
		return err
	}
	if sgid == "" {
		sgid, err = AWSCreateSecurityGroup(sess)
		if err != nil {
			return err
		}
	}

	userdata := `#!/bin/bash

# increase the max. ssh sessions
echo 'MaxSessions 1024' >> /etc/ssh/sshd_config

# set up mahimahi user
useradd -m -s /bin/bash test
usermod -aG sudo test
echo 'test ALL=(ALL) NOPASSWD: ALL' >> /etc/sudoers

# install mahimahi
apt-get update -y
apt-get install -y mahimahi
sysctl -w net.ipv4.ip_forward=1

# mount nvme
diskname=$(lsblk | grep 372 | cut -f 1 -d " ")
mkdir -m 777 /pika
mkfs -F -t ext4 /dev/$diskname
mount /dev/$diskname /pika
chmod 777 /pika

apt-get install -y chrony
sed -i '1s/^/server 169.254.169.123 prefer iburst minpoll 4 maxpoll 4\n/' /etc/chrony/chrony.conf
/etc/init.d/chrony restart`

	encodedUserdata := base64.StdEncoding.EncodeToString([]byte(userdata))

	svc := ec2.New(sess)
	config := &ec2.RunInstancesInput{
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/sda1"),
				Ebs: &ec2.EbsBlockDevice{
					VolumeSize:          aws.Int64(128),
					DeleteOnTermination: aws.Bool(true),
					VolumeType:          aws.String("gp2"),
				},
			},
		},
		ImageId:          aws.String(ami),
		InstanceType:     aws.String("c5d.4xlarge"),
		KeyName:          aws.String(keyid),
		MinCount:         aws.Int64(int64(count)),
		MaxCount:         aws.Int64(int64(count)),
		SecurityGroupIds: []*string{aws.String(sgid)},
		EbsOptimized:     aws.Bool(true),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("instance"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("pika"),
						Value: aws.String(tag),
					},
				},
			},
		},
		UserData: aws.String(encodedUserdata),
	}

	_, err = svc.RunInstances(config)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return aerr
		} else {
			return err
		}
	}

	return nil
}

func AWSStopServersAtRegion(region string, tag string) error {
	servers, err := AWSListServersAtRegion(region, tag) // TODO: don't use listservers since it does not show non-running servers
	if err != nil {
		return err
	}

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "pika"),
	})

	sid := []*string{}
	for _, s := range servers {
		sid = append(sid, aws.String(s.ID))
	}
	if len(sid) == 0 {
		return nil
	}
	svc := ec2.New(sess)
	input := &ec2.TerminateInstancesInput{
		InstanceIds: sid,
	}
	_, err = svc.TerminateInstances(input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return aerr
		} else {
			return err
		}
	}

	return nil
}

func AWSStopServersAtAllRegions(tag string) error {
	regions, err := AWSFilterRegionsByName("")
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	for _, r := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			err := AWSStopServersAtRegion(r, tag)
			if err != nil {
				fmt.Println(err)
			}
		}(r)
	}
	wg.Wait()
	return nil
}

type AWSServer struct {
	Region    string
	ID        string
	PublicIP  string
	PrivateIP string
	Tag       string
}

func AWSListServersAtAllRegions(tag string) ([]AWSServer, error) {
	regions, err := AWSFilterRegionsByName("")
	if err != nil {
		return nil, err
	}

	var res []AWSServer
	wg := &sync.WaitGroup{}
	lock := &sync.Mutex{}
	for _, r := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			d, err := AWSListServersAtRegion(r, tag)
			if err != nil {
				fmt.Println(err)
				return
			}
			lock.Lock()
			res = append(res, d...)
			lock.Unlock()
		}(r)
	}
	wg.Wait()
	return res, nil
}

func AWSListServersAtRegion(region string, tag string) ([]AWSServer, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "pika"),
	})

	svc := ec2.New(sess)

	output := []AWSServer{}

	handle := func(page *ec2.DescribeInstancesOutput, lastPage bool) bool {
		d := page.Reservations
		for _, v := range d {
			for _, instance := range v.Instances {
				// skip non-running instances
				if (*instance.State.Code)&255 != 16 {
					continue
				}
				out := AWSServer{
					ID:        *instance.InstanceId,
					Region:    region,
					Tag:       tag,
					PrivateIP: *instance.PrivateIpAddress,
					PublicIP:  *instance.PublicIpAddress,
				}
				output = append(output, out)
			}
		}
		return true
	}

	var input *ec2.DescribeInstancesInput
	if tag != "" {
		input = &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag:pika"),
					Values: []*string{
						aws.String(tag),
					},
				},
			},
		}
	} else {
		input = &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag-key"),
					Values: []*string{
						aws.String("pika"),
					},
				},
			},
		}
	}

	err = svc.DescribeInstancesPages(input, handle)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, aerr
		} else {
			return nil, err
		}
	}
	return output, nil
}

func AWSDumpServerInfo(path string, servers []AWSServer) {
	usrhome, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("error obtaining user home: ", err)
		os.Exit(1)
	}
	keypath := usrhome + "/.ssh/pikaaws"

	var dt []Server
	for _, s := range servers {
		dt = append(dt, Server{
			ID:        s.ID,
			PublicIP:  s.PublicIP,
			PrivateIP: s.PrivateIP,
			Location:  s.Region,
			Tag:       s.Tag,
			Provider:  "aws",
			User:      "ubuntu",
			KeyPath:   keypath,
			Port:      22,
		})
	}
	sort.Sort(ServerByLocation(dt))
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("error creating file: ", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	enc.Encode(dt)
}

func AWSFilterRegionsByName(location string) ([]string, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Credentials: credentials.NewSharedCredentials("", "pika"),
	})

	if err != nil {
		return nil, err
	}

	regions, err := AWSListRegions(sess)
	if err != nil {
		return nil, err
	}

	res := []string{}

	for _, v := range regions {
		if location != "" {
			loc := strings.ToLower(v.Name)
			query := strings.ToLower(location)
			if !strings.Contains(loc, query) {
				continue
			}
		}
		res = append(res, v.Name)
	}
	return res, nil
}

/*
func AWSStartServers(tag string, ec2Type string, location string, count int) []*ec2.Instance {
	// list all the regions
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)

	// Create EC2 service client
	svc := ec2.New(sess)

	// Specify the details of the instance that you want to create.
	runResult, err := svc.RunInstances(&ec2.RunInstancesInput{
		// An Amazon Linux AMI ID for t2.micro instances in the us-west-2 region
		ImageId:      aws.String("ami-e7527ed7"),
		InstanceType: aws.String("t2.micro"),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
	})

	if err != nil {
		fmt.Println("Could not create instance", err)
		return
	}

	fmt.Println("Created instance", *runResult.Instances[0].InstanceId)

	// Add tags to the created instance
	_, errtag := svc.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{runResult.Instances[0].InstanceId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String("MyFirstInstance"),
			},
		},
	})
	if errtag != nil {
		log.Println("Could not create tags for instance", runResult.Instances[0].InstanceId, errtag)
		return
	}

	fmt.Println("Successfully tagged instance")
}
*/
