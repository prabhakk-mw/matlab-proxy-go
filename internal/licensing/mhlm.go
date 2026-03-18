// Copyright 2026 The MathWorks, Inc.

package licensing

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	mwaAPIEndpoint  = "https://login.mathworks.com/authenticationws/service/v4"
	mhlmAPIEndpoint = "https://licensing.mathworks.com/mls/service/v1/entitlement/list"
	callerID        = "desktop-jupyter"
	mhlmContext     = "MATLAB_JAVASCRIPT_DESKTOP"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ExpandTokenResponse holds the result of expanding an identity token.
type ExpandTokenResponse struct {
	Expiry      string `json:"expiry"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	ProfileID   string `json:"profile_id"`
}

// AccessTokenResponse holds the result of fetching an access token.
type AccessTokenResponse struct {
	Token string `json:"token"`
}

// FetchExpandToken validates the identity token and retrieves user details
// and token expiry from the MathWorks Authentication API.
func FetchExpandToken(identityToken, sourceID string) (*ExpandTokenResponse, error) {
	form := url.Values{
		"tokenString":     {identityToken},
		"tokenPolicyName": {"R2"},
		"sourceId":        {sourceID},
	}

	req, err := http.NewRequest("POST", mwaAPIEndpoint+"/tokens", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X_MW_WS_callerId", callerID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting MathWorks auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MathWorks auth service returned %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		ExpirationDate  string `json:"expirationDate"`
		ReferenceDetail struct {
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
			DisplayName string `json:"displayName"`
			UserID      string `json:"userId"`
			ReferenceID string `json:"referenceId"`
		} `json:"referenceDetail"`
	}

	if err := decodeJSON(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("parsing expand token response: %w", err)
	}

	return &ExpandTokenResponse{
		Expiry:      data.ExpirationDate,
		FirstName:   data.ReferenceDetail.FirstName,
		LastName:    data.ReferenceDetail.LastName,
		DisplayName: data.ReferenceDetail.DisplayName,
		UserID:      data.ReferenceDetail.UserID,
		ProfileID:   data.ReferenceDetail.ReferenceID,
	}, nil
}

// FetchAccessToken exchanges an identity token for a short-lived access token.
func FetchAccessToken(identityToken, sourceID string) (*AccessTokenResponse, error) {
	form := url.Values{
		"tokenString": {identityToken},
		"type":        {"MWAS"},
		"sourceId":    {sourceID},
	}

	req, err := http.NewRequest("POST", mwaAPIEndpoint+"/tokens/access", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X_MW_WS_callerId", callerID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting MathWorks auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MathWorks auth service returned %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		AccessTokenString string `json:"accessTokenString"`
	}

	if err := decodeJSON(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("parsing access token response: %w", err)
	}

	return &AccessTokenResponse{Token: data.AccessTokenString}, nil
}

// FetchEntitlements retrieves the list of MATLAB license entitlements for the
// given access token and MATLAB release version.
func FetchEntitlements(accessToken, matlabRelease string) ([]Entitlement, error) {
	form := url.Values{
		"token":          {accessToken},
		"release":        {matlabRelease},
		"coreProduct":    {"ML"},
		"context":        {"jupyter"},
		"excludeExpired": {"true"},
	}

	req, err := http.NewRequest("POST", mhlmAPIEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting MathWorks licensing service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MathWorks licensing service returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading entitlements response: %w", err)
	}

	return parseEntitlementsXML(body)
}

// XML structures for entitlement response parsing.
type xmlEntitlementList struct {
	XMLName      xml.Name          `xml:"describe_entitlements_response"`
	Entitlements xmlEntitlementSet `xml:"entitlements"`
}

type xmlEntitlementSet struct {
	Items []xmlEntitlement `xml:"entitlement"`
}

type xmlEntitlement struct {
	ID            string `xml:"id"`
	Label         string `xml:"label"`
	LicenseNumber string `xml:"license_number"`
}

func parseEntitlementsXML(data []byte) ([]Entitlement, error) {
	var result xmlEntitlementList
	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing entitlements XML: %w", err)
	}

	items := result.Entitlements.Items
	if len(items) == 0 {
		return nil, fmt.Errorf("your MathWorks account is not linked to a valid license for this MATLAB release")
	}

	entitlements := make([]Entitlement, len(items))
	for i, item := range items {
		entitlements[i] = Entitlement{
			ID:      item.ID,
			Label:   item.Label,
			License: item.LicenseNumber,
		}
	}
	return entitlements, nil
}

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}
