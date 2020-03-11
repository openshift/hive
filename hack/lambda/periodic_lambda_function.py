#!/usr/bin/env python3

import boto3
import json
import io

def lambda_handler(event, context):

    print(event)
    running_instances = {}

    for region in event["regions"]:
        ec2client = boto3.client('ec2', region)

        response = ec2client.describe_instances()
        
        running_instances[region] = []

        for resp in response["Reservations"]:
            for instance in resp["Instances"]:
                if instance["State"]["Name"] == "running":
                    for tag in instance["Tags"]:
                        if tag["Key"] == "Name":
                            running_instances[region].append(tag["Value"])

        running_instances[region].sort()

    instances_formatted = build_instance_email_text(running_instances)

    send_email(event["recipients"], event["fromemail"], instances_formatted, event["emailregion"])

    return {
        'statusCode': 200,
        'body': json.dumps('Lambda complete!')
    }

def build_instance_email_text(instances):
    buf = io.StringIO()

    for region in instances.keys():
        buf.write("Region: {}\n".format(region))
        for instance in instances[region]:
            buf.write("{}\n".format(instance))

        buf.write("\n")
        
    return buf.getvalue()

def send_email(recipients, fromemail, email_body, region):
    sesclient = boto3.client('ses', region)

    response = sesclient.send_email(
        Source=fromemail,
        Destination={
            'ToAddresses': recipients,
        },
        Message={
            'Subject': {'Data': 'Hive running instance report'},
            'Body': {'Text': {'Data': email_body}},
        },
    )

    print("Email response: {}".format(response))

def main():
    event = {"regions": ["us-east-1", "us-east-2"],
            "recipients": ["someone@example.com"],
            "fromemail": "return@example.com",
            "emailregion": "us-east-1",
            }
    result = lambda_handler(event, "")
    print(result)

if __name__ == "__main__":
    main()
