# sig-smoke-tests-go

A stopgap quick and dirty black box test for the Swarm network / Bee client.

1/ amend config in main.go
2/ build docker container `docker build -t your-dockerhub-name/sig_smoke_tests_go:v1 .` 
3/ amend ./github/workflows/main.yml to have the correct docker reference, cron etc.
4/ push to a github repo and let the CI do it's job, check the actions tab

nb. public repos have unlimited CI minutes