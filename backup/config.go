package go_atomos

// CHECKED!

// Config Error

//var (
//	ErrConfigIsNil           = errors.New("config not found")
//	ErrConfigNodeInvalid     = errors.New("config node name is invalid")
//	ErrConfigLogPathInvalid  = errors.New("config log path is invalid")
//	ErrConfigCertPathInvalid = errors.New("config cert path is invalid")
//	ErrConfigKeyPathInvalid  = errors.New("config key path is invalid")
//)

//func (x *Config) getClientCertConfig() (tlsConfig *tls.Config, err *ErrorInfo) {
//	//cert := x.EnableCert
//	////if cert == nil {
//	////	return nil, nil
//	////}
//	////if cert.CertPath == "" {
//	////	return nil, NewError(ErrCosmosCertConfigInvalid, "Cert path is empty")
//	////}
//	//caCert, e := ioutil.ReadFile(cert.CertPath)
//	//if e != nil {
//	//	return nil, NewErrorf(ErrCosmosCertConfigInvalid, "Cert file read error, err=(%v)", e)
//	//}
//	//tlsConfig = &tls.Config{}
//	//if cert.InsecureSkipVerify {
//	//	tlsConfig.InsecureSkipVerify = true
//	//	return tlsConfig, nil
//	//}
//	//caCertPool := x509.NewCertPool()
//	//caCertPool.AppendCertsFromPEM(caCert)
//	//// Create TLS configuration with the certificate of the server.
//	//tlsConfig.RootCAs = caCertPool
//	return tlsConfig, nil
//}
//
//func (x *Config) getListenCertConfig() (tlsConfig *tls.Config, err *ErrorInfo) {
//	//cert := x.EnableCert
//	//if cert == nil {
//	//	return nil, nil
//	//}
//	//if cert.CertPath == "" {
//	//	return nil, NewError(ErrCosmosCertConfigInvalid, "Cert path is empty")
//	//}
//	//if cert.KeyPath == "" {
//	//	return nil, NewError(ErrCosmosCertConfigInvalid, "Key path is empty")
//	//}
//	//tlsConfig = &tls.Config{
//	//	Certificates: make([]tls.Certificate, 1),
//	//}
//	//var e error
//	//tlsConfig.Certificates[0], e = tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
//	//if e != nil {
//	//	return nil, NewErrorf(ErrCosmosCertConfigInvalid, "Load key pair error, err=(%v)", e)
//	//}
//	return
//}
