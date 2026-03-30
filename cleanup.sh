#!/bin/bash
set -euo pipefail

export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:?must be set}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:?must be set}"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_PAGER=""

MODE="${1:-discover}"

if [ "$MODE" = "discover" ]; then
  echo "=== Discovering agent-deploy resources ==="
  echo "--- VPCs ---"
  aws ec2 describe-vpcs --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Vpcs[].VpcId" --output text
  echo "--- Subnets ---"
  aws ec2 describe-subnets --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Subnets[].SubnetId" --output text
  echo "--- IGWs ---"
  aws ec2 describe-internet-gateways --filters "Name=tag-key,Values=agent-deploy:created-by" --query "InternetGateways[].InternetGatewayId" --output text
  echo "--- NAT GWs ---"
  aws ec2 describe-nat-gateways --filter "Name=tag-key,Values=agent-deploy:created-by" --query "NatGateways[?State!='deleted'].NatGatewayId" --output text
  echo "--- EIPs ---"
  aws ec2 describe-addresses --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Addresses[].AllocationId" --output text
  echo "--- Security Groups ---"
  aws ec2 describe-security-groups --filters "Name=tag-key,Values=agent-deploy:created-by" --query "SecurityGroups[].GroupId" --output text
  echo "--- Route Tables ---"
  aws ec2 describe-route-tables --filters "Name=tag-key,Values=agent-deploy:created-by" --query "RouteTables[].RouteTableId" --output text
  echo "--- ECS Clusters ---"
  aws ecs list-clusters --query "clusterArns" --output text
  echo "--- ALBs ---"
  aws elbv2 describe-load-balancers --query "LoadBalancers[?contains(LoadBalancerName,'agent-deploy')].LoadBalancerArn" --output text
  echo "--- Log Groups ---"
  aws logs describe-log-groups --log-group-name-prefix /ecs/agent-deploy --query "logGroups[].logGroupName" --output text
  echo "--- ECR Repos ---"
  aws ecr describe-repositories --query "repositories[?contains(repositoryName,'agent-deploy')].repositoryName" --output text
  echo "--- Lightsail Container Services ---"
  aws lightsail get-container-services --query "containerServices[].containerServiceName" --output text 2>/dev/null || echo "(no access or none found)"
  echo "--- Lightsail Certificates ---"
  aws lightsail get-certificates --query "certificates[].certificateName" --output text 2>/dev/null || echo "(no access or none found)"
  echo "--- S3 Buckets ---"
  aws s3api list-buckets --query "Buckets[?contains(Name,'agent-deploy')].Name" --output text 2>/dev/null || echo "(no access or none found)"
  echo "--- CloudFront Distributions ---"
  aws cloudfront list-distributions --query "DistributionList.Items[?Comment!=null && contains(Comment,'agent-deploy')].{Id:Id,Domain:DomainName}" --output text 2>/dev/null || echo "(no access or none found)"
  echo "--- SQS Queues ---"
  aws sqs list-queues --queue-name-prefix agent-deploy --query "QueueUrls" --output text 2>/dev/null || echo "(no access or none found)"
  echo "--- Route 53 Hosted Zones ---"
  aws route53 list-hosted-zones --query "HostedZones[].{Id:Id,Name:Name}" --output text 2>/dev/null || echo "(no access or none found)"
  echo "=== Done ==="
  exit 0
fi

if [ "$MODE" = "destroy" ]; then
  echo "=== Destroying all agent-deploy resources ==="

  # ECS services and clusters
  for cluster_arn in $(aws ecs list-clusters --query "clusterArns" --output text); do
    cluster=$(basename "$cluster_arn")
    if [[ "$cluster" == *agent-deploy* ]]; then
      for svc in $(aws ecs list-services --cluster "$cluster" --query "serviceArns" --output text 2>/dev/null); do
        echo "Deleting ECS service $svc"
        aws ecs update-service --cluster "$cluster" --service "$svc" --desired-count 0 --no-cli-pager
        aws ecs delete-service --cluster "$cluster" --service "$svc" --force --no-cli-pager
      done
      echo "Deleting ECS cluster $cluster"
      aws ecs delete-cluster --cluster "$cluster" --no-cli-pager
    fi
  done

  # ALBs, target groups, listeners
  for alb_arn in $(aws elbv2 describe-load-balancers --query "LoadBalancers[?contains(LoadBalancerName,'agent-deploy')].LoadBalancerArn" --output text); do
    for listener in $(aws elbv2 describe-listeners --load-balancer-arn "$alb_arn" --query "Listeners[].ListenerArn" --output text 2>/dev/null); do
      echo "Deleting listener $listener"
      aws elbv2 delete-listener --listener-arn "$listener"
    done
    echo "Deleting ALB $alb_arn"
    aws elbv2 delete-load-balancer --load-balancer-arn "$alb_arn"
  done
  for tg_arn in $(aws elbv2 describe-target-groups --query "TargetGroups[?contains(TargetGroupName,'agent-deploy')].TargetGroupArn" --output text); do
    echo "Deleting target group $tg_arn"
    aws elbv2 delete-target-group --target-group-arn "$tg_arn"
  done

  # ECR repos
  for repo in $(aws ecr describe-repositories --query "repositories[?contains(repositoryName,'agent-deploy')].repositoryName" --output text 2>/dev/null); do
    echo "Deleting ECR repo $repo"
    aws ecr delete-repository --repository-name "$repo" --force
  done

  # Log groups
  for lg in $(aws logs describe-log-groups --log-group-name-prefix /ecs/agent-deploy --query "logGroups[].logGroupName" --output text); do
    echo "Deleting log group $lg"
    aws logs delete-log-group --log-group-name "$lg"
  done

  # NAT Gateways
  for nat in $(aws ec2 describe-nat-gateways --filter "Name=tag-key,Values=agent-deploy:created-by" --query "NatGateways[?State!='deleted'].NatGatewayId" --output text); do
    echo "Deleting NAT Gateway $nat"
    aws ec2 delete-nat-gateway --nat-gateway-id "$nat"
  done
  echo "Waiting 30s for NAT Gateways to delete..."
  sleep 30

  # Internet Gateways
  for igw in $(aws ec2 describe-internet-gateways --filters "Name=tag-key,Values=agent-deploy:created-by" --query "InternetGateways[].InternetGatewayId" --output text); do
    vpc=$(aws ec2 describe-internet-gateways --internet-gateway-ids "$igw" --query "InternetGateways[0].Attachments[0].VpcId" --output text 2>/dev/null || echo "None")
    if [ "$vpc" != "None" ] && [ -n "$vpc" ]; then
      echo "Detaching $igw from $vpc"
      aws ec2 detach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$vpc"
    fi
    echo "Deleting IGW $igw"
    aws ec2 delete-internet-gateway --internet-gateway-id "$igw"
  done

  # Subnets
  for subnet in $(aws ec2 describe-subnets --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Subnets[].SubnetId" --output text); do
    echo "Deleting subnet $subnet"
    aws ec2 delete-subnet --subnet-id "$subnet"
  done

  # Route Tables
  for rtb in $(aws ec2 describe-route-tables --filters "Name=tag-key,Values=agent-deploy:created-by" --query "RouteTables[].RouteTableId" --output text); do
    for assoc in $(aws ec2 describe-route-tables --route-table-ids "$rtb" --query "RouteTables[0].Associations[?!Main].RouteTableAssociationId" --output text); do
      echo "Disassociating $assoc"
      aws ec2 disassociate-route-table --association-id "$assoc"
    done
    echo "Deleting route table $rtb"
    aws ec2 delete-route-table --route-table-id "$rtb"
  done

  # Security Groups
  for sg in $(aws ec2 describe-security-groups --filters "Name=tag-key,Values=agent-deploy:created-by" --query "SecurityGroups[].GroupId" --output text); do
    echo "Deleting SG $sg"
    aws ec2 delete-security-group --group-id "$sg"
  done

  # Elastic IPs
  for eip in $(aws ec2 describe-addresses --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Addresses[].AllocationId" --output text); do
    echo "Releasing EIP $eip"
    aws ec2 release-address --allocation-id "$eip"
  done

  # VPCs
  for vpc in $(aws ec2 describe-vpcs --filters "Name=tag-key,Values=agent-deploy:created-by" --query "Vpcs[].VpcId" --output text); do
    echo "Deleting VPC $vpc"
    aws ec2 delete-vpc --vpc-id "$vpc"
  done

  # IAM roles
  for role in $(aws iam list-roles --query "Roles[?starts_with(RoleName,'agent-deploy-')].RoleName" --output text 2>/dev/null); do
    for policy_arn in $(aws iam list-attached-role-policies --role-name "$role" --query "AttachedPolicies[].PolicyArn" --output text); do
      echo "Detaching $policy_arn from $role"
      aws iam detach-role-policy --role-name "$role" --policy-arn "$policy_arn"
    done
    for inline in $(aws iam list-role-policies --role-name "$role" --query "PolicyNames" --output text 2>/dev/null); do
      echo "Deleting inline policy $inline from $role"
      aws iam delete-role-policy --role-name "$role" --policy-name "$inline"
    done
    echo "Deleting IAM role $role"
    aws iam delete-role --role-name "$role"
  done

  # Lightsail container services
  for svc in $(aws lightsail get-container-services --query "containerServices[].containerServiceName" --output text 2>/dev/null); do
    echo "Deleting Lightsail container service $svc"
    aws lightsail delete-container-service --service-name "$svc"
  done

  # Lightsail certificates
  for cert in $(aws lightsail get-certificates --query "certificates[].certificateName" --output text 2>/dev/null); do
    echo "Deleting Lightsail certificate $cert"
    aws lightsail delete-certificate --certificate-name "$cert"
  done

  # S3 buckets (empty then delete)
  for bucket in $(aws s3api list-buckets --query "Buckets[?contains(Name,'agent-deploy')].Name" --output text 2>/dev/null); do
    echo "Emptying and deleting S3 bucket $bucket"
    aws s3 rm "s3://$bucket" --recursive 2>/dev/null || true
    aws s3api delete-bucket --bucket "$bucket" 2>/dev/null || true
  done

  # CloudFront distributions
  for dist_id in $(aws cloudfront list-distributions --query "DistributionList.Items[?Comment!=null && contains(Comment,'agent-deploy')].Id" --output text 2>/dev/null); do
    echo "Disabling CloudFront distribution $dist_id (must be disabled before deletion)"
    # Get current config and ETag
    etag=$(aws cloudfront get-distribution-config --id "$dist_id" --query "ETag" --output text 2>/dev/null)
    aws cloudfront get-distribution-config --id "$dist_id" --query "DistributionConfig" --output json 2>/dev/null | \
      jq '.Enabled = false' | \
      aws cloudfront update-distribution --id "$dist_id" --if-match "$etag" --distribution-config file:///dev/stdin 2>/dev/null || true
    echo "  (CloudFront distribution $dist_id disabled — manual deletion needed after it finishes deploying)"
  done

  # SQS queues
  for queue_url in $(aws sqs list-queues --queue-name-prefix agent-deploy --query "QueueUrls[]" --output text 2>/dev/null); do
    echo "Deleting SQS queue $queue_url"
    aws sqs delete-queue --queue-url "$queue_url"
  done

  echo "=== Destroy complete ==="
fi
