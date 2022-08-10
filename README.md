# awsresourcelist
Go application which fetches the resources in your AWS account and compiles them into CSV

## How to use this application?
1. Login to your AWS account and go to resource groups and tagging service
2. Go to tag editor and run a search without providing any filters
3. This will display all the [supported resources](https://docs.aws.amazon.com/ARG/latest/userguide/supported-resources.html) which should then be downloaded as a csv file
4. Run the program with the following format ./awsresourcelist -profile <profile> -region <region> -csvfile <file> <outputfile>
5. If you need any help regarding syntax, you can run ./awsresourcelist -help

## How to build the application?
1. Clone the repo
2. Navigate to the folder
3. Run the command ```go build```

You need to have Go installed to run the build command.


