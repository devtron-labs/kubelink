package registry

import (
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	client "github.com/devtron-labs/kubelink/grpc"
	"helm.sh/helm/v3/pkg/registry"
	"log"
	"os"
	"strings"
)

func extractCredentialsForRegistry(registryCredential *client.RegistryCredential) (string, string, error) {
	username := registryCredential.Username
	pwd := registryCredential.Password
	if (registryCredential.RegistryType == REGISTRYTYPE_GCR || registryCredential.RegistryType == REGISTRYTYPE_ARTIFACT_REGISTRY) && username == JSON_KEY_USERNAME {
		if strings.HasPrefix(pwd, "'") {
			pwd = pwd[1:]
		}
		if strings.HasSuffix(pwd, "'") {
			pwd = pwd[:len(pwd)-1]
		}
	}
	if registryCredential.RegistryType == REGISTRY_TYPE_ECR {
		accessKey, secretKey := registryCredential.AccessKey, registryCredential.SecretKey
		var creds *credentials.Credentials

		if len(registryCredential.AccessKey) == 0 || len(registryCredential.SecretKey) == 0 {
			sess, err := session.NewSession(&aws.Config{
				Region: &registryCredential.AwsRegion,
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
			Region:      &registryCredential.AwsRegion,
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

func OCIRegistryLogin(client *registry.Client, registryCredential *client.RegistryCredential) error {

	username, pwd, err := extractCredentialsForRegistry(registryCredential)
	if err != nil {
		return err
	}
	registryCredential.Username = username
	registryCredential.Password = pwd

	loginOptions, err := getLoginOptions(registryCredential)
	if err != nil {
		return err
	}

	err = client.Login(registryCredential.RegistryUrl,
		loginOptions...,
	)
	if err != nil {
		return err
	}

	return nil
}

func getLoginOptions(credential *client.RegistryCredential) ([]registry.LoginOption, error) {

	var loginOptions []registry.LoginOption

	if credential.Connection == SECURE_WITH_CERT_STRING {
		certificateFilePath, err := createCertificateFile(credential.RegistryName, credential.RegistryCertificate)
		if err != nil {
			return loginOptions, err
		}
		loginOptions = append(loginOptions, registry.LoginOptTLSClientConfig("", "", certificateFilePath))
	} else {
		loginOptions = append(loginOptions, registry.LoginOptBasicAuth(credential.Username, credential.Password))
	}

	allowInsecureConnection := credential.Connection == INSECURE_CONNETION_STRING

	loginOptions = append(loginOptions,
		registry.LoginOptInsecure(allowInsecureConnection))

	return loginOptions, nil
}

func createCertificateFile(registryName, caString string) (certificatePath string, err error) {

	registryFolderPath := fmt.Sprintf("%s/%s", REGISTRY_CREDENTIAL_BASE_PATH, registryName)
	certificateFilePath := fmt.Sprintf("%s/%s/ca.crt", REGISTRY_CREDENTIAL_BASE_PATH, registryName)

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
