package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
)

const (
	jsonContentType = `application/json`
)

type admitFunc func(*admission.AdmissionRequest, map[string]string) ([]patchOperation, error)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func admitFuncHandler(admit admitFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveAdmitFunc(w, r, admit)
	})
}

func isKubeNamespace(ns string) bool {
	return ns == metav1.NamespacePublic || ns == metav1.NamespaceSystem
}

func serveAdmitFunc(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	log.Print("Handling webhook request ...")

	var writeErr error
	if bytes, err := doServeAdmitFunc(w, r, admit); err != nil {
		log.Printf("Error handling webhook request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, writeErr = w.Write([]byte(err.Error()))
	} else {
		log.Print("Webhook request handled successfully")
		_, writeErr = w.Write(bytes)
	}

	if writeErr != nil {
		log.Printf("Could not write response: %v", writeErr)
	}
}

func doServeAdmitFunc(w http.ResponseWriter, r *http.Request, admit admitFunc) ([]byte, error) {
	// Step 1: Request validation. Only handle POST requests with a body and json content type.

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil, fmt.Errorf("invalid method %s, only POST requests are allowed", r.Method)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("could not read request body: %v", err)
	}

	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("unsupported content type %s, only %s is supported", contentType, jsonContentType)
	}

	// Step 2: Parse the AdmissionReview request.

	var admissionReviewReq admission.AdmissionReview

	if _, _, err := universalDeserializer.Decode(body, nil, &admissionReviewReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("could not deserialize request: %v", err)
	} else if admissionReviewReq.Request == nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, errors.New("malformed admission review: request is nil")
	}

	// Step 3: Construct the AdmissionReview response.

	admissionReviewResponse := admission.AdmissionReview{
		// Since the admission webhook now supports multiple API versions, we need
		// to explicitly include the API version in the response.
		// This API version needs to match the version from the request exactly, otherwise
		// the API server will be unable to process the response.
		// Note: a v1beta1 AdmissionReview is JSON-compatible with the v1 version, that's why
		// we do not need to differentiate during unmarshaling or in the actual logic.
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &admission.AdmissionResponse{
			UID: admissionReviewReq.Request.UID,
		},
	}

	var patchOps []patchOperation
	// Apply the admit() function only for non-Kubernetes namespaces. For objects in Kubernetes namespaces, return
	// an empty set of patch operations.
	if !isKubeNamespace(admissionReviewReq.Request.Namespace) {
		annotations := make(map[string]string)
		annotations["sidecar.winkyi"] = "hello"
		patchOps, err = admit(admissionReviewReq.Request, annotations)
	}

	if err != nil {
		// If the handler returned an error, incorporate the error message into the response and deny the object
		// creation.
		admissionReviewResponse.Response.Allowed = false
		admissionReviewResponse.Response.Result = &metav1.Status{
			Message: err.Error(),
		}
	} else {
		// Otherwise, encode the patch operations to JSON and return a positive response.
		patchBytes, err := json.Marshal(patchOps)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return nil, fmt.Errorf("could not marshal JSON patch: %v", err)
		}
		admissionReviewResponse.Response.Allowed = true
		admissionReviewResponse.Response.Patch = patchBytes

		// Announce that we are returning a JSON patch (note: this is the only
		// patch type currently supported, but we have to explicitly announce
		// it nonetheless).
		admissionReviewResponse.Response.PatchType = new(admission.PatchType)
		*admissionReviewResponse.Response.PatchType = admission.PatchTypeJSONPatch
	}

	// Return the AdmissionReview with a response as JSON.
	bytes, err := json.Marshal(&admissionReviewResponse)
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %v", err)
	}
	return bytes, nil
}
