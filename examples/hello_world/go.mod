module "github.com/hwangtou/go-atomos/examples"

go 1.16

require (
	"github.com/hwangtou/go-atomos" v1.0.0
	"github.com/hwangtou/go-atomos/examples/hello_world/api" v1.0.0
	"github.com/golang/protobuf" v1.5.2
)
replace (
	"github.com/hwangtou/go-atomos" => "../../"
	"github.com/hwangtou/go-atomos/examples/hello_world/api" => "./api/"
)
