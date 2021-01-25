bee --config /go/src/app/bee-staging.yml start &> /tmp/bee.log &
head -n100 /tmp/bee.log

while ! nc -vz localhost 1633
do
  sleep 5
  tail -n5 /tmp/bee.log
done

go get -d -v ./...
go install -v ./...
app

echo "------ END OF TEST OUTPUT"

cat /tmp/bee.log
