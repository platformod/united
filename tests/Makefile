test:
	terraform init
	terraform apply -auto-approve
	terraform apply -var changer=bar -auto-approve
	terraform state pull
	terraform destroy -auto-approve
