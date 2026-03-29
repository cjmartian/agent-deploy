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
    echo "Deleting IAM role $role"
    aws iam delete-role --role-name "$role"
  done

  echo "=== Destroy complete ==="
fi
