
thrift:
	thrift -r -gen go:package_prefix=github.com/yu1ec/go-impala-beeswax/services/ interfaces/ImpalaService.thrift
	rm -rf ./services
	mv gen-go services
