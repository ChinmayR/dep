package galileotest

//go:generate mockgen -package galileotest -destination mock_galileo.go code.uber.internal/engsec/galileo-go.git Galileo
//go:generate sed -i "" -e s|code.uber.internal/engsec/galileo-go.git/vendor/|| mock_galileo.go
