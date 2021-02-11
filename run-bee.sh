/go/src/github.com/ethersphere/bee/dist/bee --config /go/src/app/bee-staging.yml start &> /tmp/bee.log &
#sleep 30
until [ -f /go/src/app/data/keys/swarm.key ]; do echo "waiting for bee"; sleep 3; done
ADDRE=$(cat /go/src/app/data/keys/swarm.key | jq .address | tr -d \")
echo $ADDRE

curl -s -XPOST https://faucet.ethswarm.org/fund-gbzz --data token\=$FAUCET_TOKEN\&receiver\=$ADDRE

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


