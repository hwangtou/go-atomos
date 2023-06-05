package go_atomos

//func TestAppUDSServer(t *testing.T) {
//	appLoggingTestingT = t
//	listener, err := testAppUDSServer(t)
//	if err != nil {
//		t.Errorf("TestAppUDSServer: Failed. err=%v", err)
//		return
//	}
//	defer listener.close()
//
//	client := testAppSocketClient(t)
//	conn, err := client.Dial(listener.addr.Name)
//	if err != nil {
//		t.Errorf("TestAppUDSServer: Dial failed. err=%v", err)
//		return
//	}
//	defer client.Close()
//
//	client.Send(conn, UDSPing, []byte("ping"), 1*time.Second, func(packet *UDSCommandPacket, e *Error) {
//		if err != nil {
//			t.Errorf("TestAppUDSServer: Ping returns failed. err=%v", err)
//			return
//		}
//		t.Logf("TestAppUDSServer: Ping. response=(%+v),err=(%v)", packet, e)
//	})
//
//	//client.Send(conn, UDSNodeHalt, []byte("halt"), 1*time.Second, func(packet *UDSCommandPacket, e *Error) {
//	//	if err != nil {
//	//		t.Errorf("TestAppUDSServer: NodeHalt returns failed. err=%v", err)
//	//		return
//	//	}
//	//	t.Logf("TestAppUDSServer: NodeHalt. response=(%+v),err=(%v)", packet, e)
//	//})
//
//	<-time.After(100 * time.Millisecond)
//}
