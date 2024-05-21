package registry

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/registry"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
)

func OCIRegistryLogin(client *registry.Client, config *Configuration) error {

	username, pwd, err := extractCredentialsForRegistry(config)
	if err != nil {
		return err
	}
	config.Username = username
	config.Password = pwd

	loginOptions, err := getLoginOptions(config)
	if err != nil {
		return err
	}

	err = client.Login(config.RegistryUrl,
		loginOptions...,
	)
	if err != nil {
		return err
	}

	return nil
}

func CreateCertificateFile(registryName, caString string) (certificatePath string, err error) {

	registryFolderPath := fmt.Sprintf("%s/%s-%v", REGISTRY_CREDENTIAL_BASE_PATH, registryName, rand.Int())
	certificateFilePath := fmt.Sprintf("%s/ca.crt", registryFolderPath)

	if _, err = os.Stat(certificateFilePath); os.IsExist(err) {
		// if file exists - remove file
		err := os.Remove(certificateFilePath)
		if err != nil {
			return certificatePath, err
		}
	} else if _, err = os.Stat(registryFolderPath); os.IsNotExist(err) {
		// create folder if not exist
		err = os.MkdirAll(registryFolderPath, os.ModePerm)
		if err != nil {
			return certificatePath, err
		}
	}
	f, err := os.Create(certificateFilePath)
	if err != nil {
		return certificatePath, err
	}
	defer f.Close()
	_, err2 := f.WriteString(caString)
	if err2 != nil {
		return certificatePath, err
	}
	return certificateFilePath, nil
}

func extractCredentialsForRegistry(config *Configuration) (string, string, error) {
	username := config.Username
	pwd := config.Password
	if (config.RegistryType == REGISTRYTYPE_GCR || config.RegistryType == REGISTRYTYPE_ARTIFACT_REGISTRY) && username == JSON_KEY_USERNAME {
		if strings.HasPrefix(pwd, "'") {
			pwd = pwd[1:]
		}
		if strings.HasSuffix(pwd, "'") {
			pwd = pwd[:len(pwd)-1]
		}
	}
	if config.RegistryType == REGISTRY_TYPE_ECR {
		accessKey, secretKey := config.AwsAccessKey, config.AwsSecretKey
		var creds *credentials.Credentials

		if len(config.AwsAccessKey) == 0 || len(config.AwsSecretKey) == 0 {
			sess, err := session.NewSession(&aws.Config{
				Region: &config.AwsRegion,
			})
			if err != nil {
				log.Printf("error in creating AWS client %w ", err)
				return "", "", err
			}
			creds = ec2rolecreds.NewCredentials(sess)
		} else {
			creds = credentials.NewStaticCredentials(accessKey, secretKey, "")
		}
		sess, err := session.NewSession(&aws.Config{
			Region:      &config.AwsRegion,
			Credentials: creds,
		})
		if err != nil {
			log.Printf("error in creating AWS client %w ", err)
			return "", "", err
		}
		svc := ecr.New(sess)
		input := &ecr.GetAuthorizationTokenInput{}
		authData, err := svc.GetAuthorizationToken(input)
		if err != nil {
			log.Printf("error in creating AWS client %w ", err)
			return "", "", err
		}
		// decode token
		token := authData.AuthorizationData[0].AuthorizationToken
		decodedToken, err := base64.StdEncoding.DecodeString(*token)
		if err != nil {
			log.Printf("error in creating AWS client %w ", err)
			return "", "", err
		}
		credsSlice := strings.Split(string(decodedToken), ":")
		username = credsSlice[0]
		pwd = credsSlice[1]

	}
	return username, pwd, nil
}

func getLoginOptions(config *Configuration) ([]registry.LoginOption, error) {

	var loginOptions []registry.LoginOption

	loginOptions = append(loginOptions, registry.LoginOptBasicAuth(config.Username, config.Password))

	isSecureConnection := config.RegistryConnectionType == INSECURE_CONNECTION

	loginOptions = append(loginOptions,
		registry.LoginOptInsecure(isSecureConnection))

	if !isSecureConnection && config.RegistryConnectionType == SECURE_WITH_CERT {
		loginOptions = append(loginOptions, registry.LoginOptTLSClientConfig("", "", config.RegistryCAFilePath))
	}

	return loginOptions, nil
}

func GetHttpClient(config *Configuration) (*http.Client, error) {
	tlsConfig, err := GetTlsConfig(config)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}
	return httpClient, nil
}

func GetTlsConfig(config *Configuration) (*tls.Config, error) {
	isInsecure := config.RegistryConnectionType == INSECURE_CONNECTION
	tlsConfig, err := NewClientTLS("", "", config.RegistryCAFilePath, isInsecure)
	if err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

// TODO: add support for certFile, keyFile on UI?
func NewClientTLS(certFile, keyFile, caFile string, insecureSkipTLSverify bool) (*tls.Config, error) {
	config := tls.Config{
		InsecureSkipVerify: insecureSkipTLSverify,
	}

	if certFile != "" && keyFile != "" {
		cert, err := CertFromFilePair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{*cert}
	}

	if caFile != "" {
		cp, err := CertPoolFromFile(caFile)
		if err != nil {
			return nil, err
		}
		config.RootCAs = cp
	}

	return &config, nil
}

func CertFromFilePair(certFile, keyFile string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "can't load key pair from cert %s and key %s", certFile, keyFile)
	}
	return &cert, err
}

func CertPoolFromFile(filename string) (*x509.CertPool, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Errorf("can't read CA file: %v", filename)
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		return nil, errors.Errorf("failed to append certificates from file: %s", filename)
	}
	return cp, nil
}
