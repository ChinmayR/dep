package httpserver

//go:generate mockgen -destination mock_test.go -package httpserver net Conn,Listener
