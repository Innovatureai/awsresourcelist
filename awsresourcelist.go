package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

type awscfnresource struct {
	PhysicalResourceId string
	LogicalResourceId  string
}

// Get all the resources from the Resource Group Tagging API

func getallcfnresources(client *cloudformation.Client, stackID string) []awscfnresource {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// Get the list of resources from the CloudFormation API
	awscfnresourceslice := []awscfnresource{}
	output, err := client.DescribeStackResources(context.TODO(), &cloudformation.DescribeStackResourcesInput{
		StackName: aws.String(stackID),
	})
	if err != nil {
		sugar.Fatalf("ListStackResources Error: %v", err)
	}
	for _, object := range output.StackResources {
		sugar.Debugf("ARN=%s", aws.ToString(object.LogicalResourceId))
		awscfnresourceslice = append(awscfnresourceslice, awscfnresource{*object.PhysicalResourceId, *object.LogicalResourceId})
		iscloudformationARN, err := regexp.MatchString("^arn:aws:cloudformation:", *object.PhysicalResourceId)
		if iscloudformationARN && err == nil {
			sugar.Debugf("Found the nested stack")
			awscfnresourceslice = append(awscfnresourceslice, getallcfnresources(client, *object.PhysicalResourceId)...)
		} else {
			sugar.Debugf("Not clouformation stack ARN")
		}
	}
	return awscfnresourceslice
}

func loadcsv(csvfile string) [][]string {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// Load the CSV file
	csvfilehandle, err := os.Open(csvfile)
	if err != nil {
		sugar.Fatalf("Error opening CSV file: %v", err)
	}
	defer csvfilehandle.Close()
	reader := csv.NewReader(csvfilehandle)
	reader.Comma = ','
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	record, err := reader.ReadAll()
	if err != nil {
		sugar.Fatalf("Error reading CSV file: %v", err)
	}
	sugar.Debugf("record %v", record)
	return record
}

func searchfromrecord(record [][]string, search string) ([]string, int) {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// Search the CSV file for the search string
	for countrecord, row := range record {
		issearcharn, err := regexp.MatchString("^arn:.+$", search)
		if issearcharn && err == nil {
			// remove the arn: prefix
			ss := strings.Split(search, ":")
			search = ss[len(ss)-1]
			isstack, err := regexp.MatchString("^stack/", search)
			if isstack && err == nil {
				// remove the stack/ prefix
				ss := strings.Split(search, "/")
				search = ss[len(ss)-2] + "/" + ss[len(ss)-1]
			}
			sugar.Debugf("Arn search %s", search)
		}
		ifMatch, err := regexp.MatchString(search, row[0])
		if row[0] == search || ifMatch && err == nil {
			sugar.Debugf("Found %s", search)
			return row, countrecord
		} else {
			sugar.Debugf("Not found %s", search)
		}
	}
	return nil, -1
}

func removesliceentry(slice [][]string, i int) [][]string {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// Remove the entry from the slice
	sugar.Debugf("Removing entry %d", i)
	slice = append(slice[:i], slice[i+1:]...)
	return slice
}

func findallcloudformationstacks(client *cloudformation.Client, paginationToken string) []string {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// Get the list of resources from the CloudFormation API
	stacklist := []string{}
	var output *cloudformation.ListStacksOutput

	if paginationToken == "" {
		output, err = client.ListStacks(context.TODO(), &cloudformation.ListStacksInput{
			StackStatusFilter: []types.StackStatus{types.StackStatusCreateComplete, types.StackStatusUpdateComplete, types.StackStatusUpdateRollbackComplete},
		})
		if err != nil {
			sugar.Fatalf("ListStackResources Error: %v", err)
		}
	} else {
		output, err = client.ListStacks(context.TODO(), &cloudformation.ListStacksInput{
			NextToken: aws.String(paginationToken),
		})
		if err != nil {
			sugar.Fatalf("ListStackResources Error: %v", err)
		}
	}

	for _, object := range output.StackSummaries {
		sugar.Debugf("StackID: %s", aws.ToString(object.StackId))
		sugar.Debugf("ParentId: %s", aws.ToString(object.ParentId))
		if object.ParentId == nil {
			stacklist = append(stacklist, aws.ToString(object.StackId))
		}
	}
	if output.NextToken != nil {
		stacklist = append(stacklist, findallcloudformationstacks(client, aws.ToString(output.NextToken))...)
	}
	return stacklist
}

func findalliamroles(client *iam.Client, wg *sync.WaitGroup, iamrolelistchan chan<- [][]string) {
	defer close(iamrolelistchan)
	defer wg.Done()
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Infof("Finding all IAM roles function running")
	// Get the list of resources from the IAM API
	rolelist := [][]string{}
	var output *iam.ListRolesOutput

	output, err = client.ListRoles(context.TODO(), &iam.ListRolesInput{})
	if err != nil {
		sugar.Fatalf("ListRoles Error: %v", err)
	}
	var singlerole []string = []string{}

	for _, object := range output.Roles {
		sugar.Debugf("Role: %s", aws.ToString(object.RoleName))
		singlerole = []string{aws.ToString(object.RoleName), aws.ToString(object.RoleId), aws.ToString(object.Arn), "IAM", "Role", ""}
		rolelist = append(rolelist, singlerole)
	}
	iamrolelistchan <- rolelist
	sugar.Infof("Finding all IAM roles function complete")

}

func findallcloudwatchlogsloggroups(client *cloudwatchlogs.Client, wg *sync.WaitGroup, loglistchan chan<- [][]string) {
	defer close(loglistchan)
	defer wg.Done()
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Infof("Finding all CloudWatch Logs log groups function running")
	// Get the list of resources from the CloudWatchLogs API
	loggrouplist := [][]string{}
	var output *cloudwatchlogs.DescribeLogGroupsOutput

	output, err = client.DescribeLogGroups(context.TODO(), &cloudwatchlogs.DescribeLogGroupsInput{})
	if err != nil {
		sugar.Fatalf("DescribeLogGroups Error: %v", err)
	}
	var singlegroup []string = []string{}

	for _, object := range output.LogGroups {
		sugar.Debugf("LogGroup: %s", aws.ToString(object.LogGroupName))
		singlegroup = []string{aws.ToString(object.LogGroupName), aws.ToString(object.Arn), "CloudWatchLogs", "LogGroup", ""}
		loggrouplist = append(loggrouplist, singlegroup)
	}
	loglistchan <- loggrouplist
	sugar.Infof("Finding all CloudWatch Logs log groups function complete")
}

func main() {
	// Initialise zap logging
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	// flag section
	var awsProfile string
	var awsRegion string
	var fileName string
	var loadFileName string
	flag.StringVar(&awsProfile, "profile", "", "AWS profile to use")
	flag.StringVar(&awsRegion, "region", "", "AWS region to use (Only required if profile without default region is specified)")
	flag.StringVar(&loadFileName, "csvfile", "", "File to load taken from resource group")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	//print usage if no arguments are provided
	if (len(os.Args) <= 1) || *help {
		fmt.Print("Please use the following usage instructions while using this command:\n\nIf you have any question, reach out to innovature.ai \n\n")
		flag.PrintDefaults()
		fmt.Print("\n\nIf you would like to mention the output file, you can do so as an argument.\n\nEg: ./awsresourcelist -profile <profile> -region <region> -csvfile <file> <outputfile>\n\n")
		os.Exit(1)
	}

	// Initialise the file name
	if len(os.Args) >= 2 {
		fileName = os.Args[len(os.Args)-1]
	} else if fileName == "" && awsProfile != "" {
		fileName = "output-resources.csv"
	}

	if loadFileName == "" {
		sugar.Fatalf("No CSV file specified")
	}

	// Initialise the file
	csvFile, err := os.Create(fileName)
	if err != nil {
		sugar.Fatalf("Failed creating file %s: %s", fileName, err)
	}
	csvwriter := csv.NewWriter(csvFile)
	_ = csvwriter.Write([]string{"Sl.No.", "ARN/Resource ID", "LogicalID", "Name", "Service", "Type", "Region"})
	if err != nil {
		sugar.Fatal(err)
	}
	// Load the Shared AWS Configuration (~/.aws/config)
	var cfg aws.Config
	if awsProfile != "" {
		sugar.Debugf("Using profile %s", awsProfile)
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithSharedConfigProfile(awsProfile))
		if err != nil {
			sugar.Fatal(err)
		}
	} else if awsProfile != "" && awsRegion != "" {
		sugar.Debugf("Using default cred chain")
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithSharedConfigProfile(awsProfile), config.WithRegion(awsRegion))
		if err != nil {
			sugar.Fatal(err)
		}
	} else if awsRegion != "" {
		sugar.Debugf("Using default cred chain")
		cfg = aws.Config{
			Region: awsRegion,
		}
	} else {
		sugar.Fatalf("No profile or region specified")
	}

	// Create service clients
	sugar.Infof("Starting service client creation")

	cfnclient := cloudformation.NewFromConfig(cfg)
	iamclient := iam.NewFromConfig(cfg)
	logclient := cloudwatchlogs.NewFromConfig(cfg)

	sugar.Infof("Service client creation complete")

	stacklist := findallcloudformationstacks(cfnclient, "")

	sugar.Infof("Obtained all CloudFormation stacks and resources")

	loglistchan := make(chan [][]string, 1000000)
	iamrolelistchan := make(chan [][]string, 1000000)
	sugar.Infof("Initialised channels")

	// Start the goroutines
	var wg sync.WaitGroup
	sugar.Infof("Starting goroutines")
	wg.Add(2)
	go findallcloudwatchlogsloggroups(logclient, &wg, loglistchan)
	go findalliamroles(iamclient, &wg, iamrolelistchan)
	sugar.Infof("Waiting for goroutines to finish")

	wg.Wait()
	iamrolelist := <-iamrolelistchan
	loglist := <-loglistchan

	sugar.Infof("All goroutines complete")

	sugar.Infof("Converting channels to slices")

	sugar.Infof("Converting channels to slices complete")
	sugar.Debugf("Stacklist %v", stacklist)

	// Cloudformation section

	// CSV Load section
	record := loadcsv(loadFileName)

	sugar.Debugf("%v", record)

	// Record Search section

	_ = csvwriter.Write([]string{"a", fmt.Sprintf("Resource list from Cloudformation template")})
	for countstack, stackId := range stacklist {
		sugar.Debugf("Stack %s", stackId)

		cfnsearchresult, countrecord := searchfromrecord(record, stackId)
		if cfnsearchresult != nil {
			_ = csvwriter.Write([]string{fmt.Sprint(countstack + 1), stackId, "", cfnsearchresult[1], cfnsearchresult[2], cfnsearchresult[3], cfnsearchresult[4]})

			record = removesliceentry(record, countrecord)
		} else {
			sugar.Debugf("Not found %s", stackId)
			_ = csvwriter.Write([]string{fmt.Sprint(countstack + 1), stackId, "", "", "", "", ""})
		}
		awscfnresourceslice := getallcfnresources(cfnclient, stackId)
		sugar.Debugf("%v", awscfnresourceslice)
		count := 0
		for countcfn, resource := range awscfnresourceslice {

			// check if the resource id exists in the csv record
			searchresult, countrecord := searchfromrecord(record, resource.PhysicalResourceId)
			if searchresult != nil {
				sugar.Debugf("%v", searchresult)

				record = removesliceentry(record, countrecord)
				_ = csvwriter.Write([]string{fmt.Sprintf("%d.%d", countstack+1, count+1), resource.PhysicalResourceId, resource.LogicalResourceId, searchresult[1], searchresult[2], searchresult[3], searchresult[4]})
				count++
				// check if the resource id exists in the role list
			} else if searchresult == nil {
				iamsearchresult, iamcountrecord := searchfromrecord(iamrolelist, resource.PhysicalResourceId)
				if iamsearchresult != nil {
					sugar.Debugf("%v", iamsearchresult)

					iamrolelist = removesliceentry(iamrolelist, iamcountrecord)
					_ = csvwriter.Write([]string{fmt.Sprintf("%d.%d", countstack+1, count+1), resource.PhysicalResourceId, resource.LogicalResourceId, iamsearchresult[1], iamsearchresult[2], iamsearchresult[3], iamsearchresult[4]})
					count++
					// check if the resource id exists in the log group list
				} else {
					logsearchresult, logcountrecord := searchfromrecord(loglist, resource.PhysicalResourceId)
					if logsearchresult != nil {
						sugar.Debugf("%v", logsearchresult)

						loglist = removesliceentry(loglist, logcountrecord)
						_ = csvwriter.Write([]string{fmt.Sprintf("%d.%d", countstack+1, count+1), resource.PhysicalResourceId, resource.LogicalResourceId, logsearchresult[1], logsearchresult[2], logsearchresult[3], logsearchresult[4]})
						count++
					}
				}
				// if it doesn't exist in the csv record, then just directly print it without adding any additional information
			} else {
				_ = csvwriter.Write([]string{fmt.Sprintf("%d.%d", countstack+1, countcfn+1), resource.PhysicalResourceId, resource.LogicalResourceId, "", "", "", cfnsearchresult[4]})
			}

			sugar.Debugf("%s,%s\n", resource.PhysicalResourceId, resource.LogicalResourceId)
			csvwriter.Flush()
			if err != nil {
				sugar.Fatal(err)
			}
		}

	}
	_ = csvwriter.Write([]string{"b", fmt.Sprintf("Non cloudformation linked resource list from CSV file")})

	// add all the non cloudformation linked resources in the csv record to the csv file

	for countrecord, records := range record {
		_ = csvwriter.Write([]string{fmt.Sprint(countrecord + 1), records[0], "", records[1], records[2], records[3], records[4], records[5]})
		csvwriter.Flush()
		if err != nil {
			sugar.Fatal(err)
		}
	}

	_ = csvwriter.Write([]string{"c", fmt.Sprintf("Non cloudformation linked IAM roles")})

	// add all the non cloudformation linked IAM roles to the csv file

	for countiamrecord, iamrecords := range iamrolelist {
		_ = csvwriter.Write([]string{fmt.Sprint(countiamrecord + 1), iamrecords[0], iamrecords[1], iamrecords[2], iamrecords[3], iamrecords[4], iamrecords[5]})
		csvwriter.Flush()
		if err != nil {
			sugar.Fatal(err)
		}
	}

	_ = csvwriter.Write([]string{"d", fmt.Sprintf("Non cloudformation linked Cloudwatch logs")})

	// add all the non cloudformation linked Cloudwatch logs to the csv file

	for countlogrecord, logrecords := range loglist {
		_ = csvwriter.Write([]string{fmt.Sprint(countlogrecord + 1), logrecords[0], logrecords[1], logrecords[2], logrecords[3], logrecords[4]})
		csvwriter.Flush()
		if err != nil {
			sugar.Fatal(err)
		}
	}

	csvFile.Close()

}
