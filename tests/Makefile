.EXPORT_ALL_VARIABLES:
TF_HTTP_USERNAME?=foo
TF_HTTP_PASSWORD?=f00f00f00
AWS_PROFILE?=localstack

test:
	terraform init
	terraform apply -auto-approve
	terraform apply -var changer=bar -auto-approve
	terraform state pull
	terraform destroy -auto-approve
	terraform state pull

	curl -qv -fsSL --user ${TF_HTTP_USERNAME}:${TF_HTTP_PASSWORD} -X DELETE http://localhost:8080/state/my-peoples/me
	@echo
	PAGER=cat aws s3api list-objects --bucket united-test
