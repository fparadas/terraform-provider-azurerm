package apimanagement

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/apimanagement/mgmt/2020-12-01/apimanagement"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/apimanagement/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/apimanagement/schemaz"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/apimanagement/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceApiManagementDiagnostic() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceApiManagementDiagnosticCreateUpdate,
		Read:   resourceApiManagementDiagnosticRead,
		Update: resourceApiManagementDiagnosticCreateUpdate,
		Delete: resourceApiManagementDiagnosticDelete,

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.DiagnosticID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"identifier": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"applicationinsights",
					"azuremonitor",
				}, false),
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"api_management_name": schemaz.SchemaApiManagementName(),

			"api_management_logger_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: validate.LoggerID,
			},

			"enabled": {
				Type:       pluginsdk.TypeBool,
				Optional:   true,
				Deprecated: "this property has been removed from the API and will be removed in version 3.0 of the provider",
			},

			"sampling_percentage": {
				Type:         pluginsdk.TypeFloat,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.FloatBetween(0.0, 100.0),
			},

			"always_log_errors": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				Computed: true,
			},

			"verbosity": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(apimanagement.Verbose),
					string(apimanagement.Information),
					string(apimanagement.Error),
				}, false),
			},

			"log_client_ip": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				Computed: true,
			},

			"http_correlation_protocol": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(apimanagement.HTTPCorrelationProtocolNone),
					string(apimanagement.HTTPCorrelationProtocolLegacy),
					string(apimanagement.HTTPCorrelationProtocolW3C),
				}, false),
			},

			"frontend_request": resourceApiManagementApiDiagnosticAdditionalContentSchema(),

			"frontend_response": resourceApiManagementApiDiagnosticAdditionalContentSchema(),

			"backend_request": resourceApiManagementApiDiagnosticAdditionalContentSchema(),

			"backend_response": resourceApiManagementApiDiagnosticAdditionalContentSchema(),

			"operation_name_format": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Default:  string(apimanagement.Name),
				ValidateFunc: validation.StringInSlice([]string{
					string(apimanagement.Name),
					string(apimanagement.URL),
				}, false),
			},
		},
	}
}

func resourceApiManagementDiagnosticCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.DiagnosticClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	diagnosticId := d.Get("identifier").(string)
	resourceGroup := d.Get("resource_group_name").(string)
	serviceName := d.Get("api_management_name").(string)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, serviceName, diagnosticId)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing Diagnostic %q (API Management Service %q / Resource Group %q): %s", diagnosticId, serviceName, resourceGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_api_management_diagnostic", *existing.ID)
		}
	}

	parameters := apimanagement.DiagnosticContract{
		DiagnosticContractProperties: &apimanagement.DiagnosticContractProperties{
			LoggerID:            utils.String(d.Get("api_management_logger_id").(string)),
			OperationNameFormat: apimanagement.OperationNameFormat(d.Get("operation_name_format").(string)),
		},
	}

	if samplingPercentage, ok := d.GetOk("sampling_percentage"); ok {
		parameters.Sampling = &apimanagement.SamplingSettings{
			SamplingType: apimanagement.Fixed,
			Percentage:   utils.Float(samplingPercentage.(float64)),
		}
	} else {
		parameters.Sampling = nil
	}

	if alwaysLogErrors, ok := d.GetOk("always_log_errors"); ok && alwaysLogErrors.(bool) {
		parameters.AlwaysLog = apimanagement.AllErrors
	}

	if verbosity, ok := d.GetOk("verbosity"); ok {
		parameters.Verbosity = apimanagement.Verbosity(verbosity.(string))
	}

	if logClientIP, exists := d.GetOkExists("log_client_ip"); exists { //nolint:SA1019
		parameters.LogClientIP = utils.Bool(logClientIP.(bool))
	}

	if httpCorrelationProtocol, ok := d.GetOk("http_correlation_protocol"); ok {
		parameters.HTTPCorrelationProtocol = apimanagement.HTTPCorrelationProtocol(httpCorrelationProtocol.(string))
	}

	frontendRequest, frontendRequestSet := d.GetOk("frontend_request")
	frontendResponse, frontendResponseSet := d.GetOk("frontend_response")
	if frontendRequestSet || frontendResponseSet {
		parameters.Frontend = &apimanagement.PipelineDiagnosticSettings{}
		if frontendRequestSet {
			parameters.Frontend.Request = expandApiManagementApiDiagnosticHTTPMessageDiagnostic(frontendRequest.([]interface{}))
		}
		if frontendResponseSet {
			parameters.Frontend.Response = expandApiManagementApiDiagnosticHTTPMessageDiagnostic(frontendResponse.([]interface{}))
		}
	}

	backendRequest, backendRequestSet := d.GetOk("backend_request")
	backendResponse, backendResponseSet := d.GetOk("backend_response")
	if backendRequestSet || backendResponseSet {
		parameters.Backend = &apimanagement.PipelineDiagnosticSettings{}
		if backendRequestSet {
			parameters.Backend.Request = expandApiManagementApiDiagnosticHTTPMessageDiagnostic(backendRequest.([]interface{}))
		}
		if backendResponseSet {
			parameters.Backend.Response = expandApiManagementApiDiagnosticHTTPMessageDiagnostic(backendResponse.([]interface{}))
		}
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroup, serviceName, diagnosticId, parameters, ""); err != nil {
		return fmt.Errorf("creating or updating Diagnostic %q (Resource Group %q / API Management Service %q): %+v", diagnosticId, resourceGroup, serviceName, err)
	}

	resp, err := client.Get(ctx, resourceGroup, serviceName, diagnosticId)
	if err != nil {
		return fmt.Errorf("retrieving Diagnostic %q (Resource Group %q / API Management Service %q): %+v", diagnosticId, resourceGroup, serviceName, err)
	}
	if resp.ID == nil {
		return fmt.Errorf("reading ID for Diagnostic %q (Resource Group %q / API Management Service %q): ID is empty", diagnosticId, resourceGroup, serviceName)
	}
	d.SetId(*resp.ID)

	return resourceApiManagementDiagnosticRead(d, meta)
}

func resourceApiManagementDiagnosticRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.DiagnosticClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	diagnosticId, err := parse.DiagnosticID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, diagnosticId.ResourceGroup, diagnosticId.ServiceName, diagnosticId.Name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] Diagnostic %q (Resource Group %q / API Management Service %q) was not found - removing from state!", diagnosticId.Name, diagnosticId.ResourceGroup, diagnosticId.ServiceName)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("making Read request for Diagnostic %q (Resource Group %q / API Management Service %q): %+v", diagnosticId.Name, diagnosticId.ResourceGroup, diagnosticId.ServiceName, err)
	}

	d.Set("identifier", resp.Name)
	d.Set("resource_group_name", diagnosticId.ResourceGroup)
	d.Set("api_management_name", diagnosticId.ServiceName)
	d.Set("api_management_logger_id", resp.LoggerID)
	if props := resp.DiagnosticContractProperties; props != nil {
		if props.Sampling != nil && props.Sampling.Percentage != nil {
			d.Set("sampling_percentage", props.Sampling.Percentage)
		}
		d.Set("always_log_errors", props.AlwaysLog == apimanagement.AllErrors)
		d.Set("verbosity", props.Verbosity)
		d.Set("log_client_ip", props.LogClientIP)
		d.Set("http_correlation_protocol", props.HTTPCorrelationProtocol)
		if frontend := props.Frontend; frontend != nil {
			d.Set("frontend_request", flattenApiManagementApiDiagnosticHTTPMessageDiagnostic(frontend.Request))
			d.Set("frontend_response", flattenApiManagementApiDiagnosticHTTPMessageDiagnostic(frontend.Response))
		} else {
			d.Set("frontend_request", nil)
			d.Set("frontend_response", nil)
		}
		if backend := props.Backend; backend != nil {
			d.Set("backend_request", flattenApiManagementApiDiagnosticHTTPMessageDiagnostic(backend.Request))
			d.Set("backend_response", flattenApiManagementApiDiagnosticHTTPMessageDiagnostic(backend.Response))
		} else {
			d.Set("backend_request", nil)
			d.Set("backend_response", nil)
		}
		format := string(apimanagement.Name)
		if props.OperationNameFormat != "" {
			format = string(props.OperationNameFormat)
		}
		d.Set("operation_name_format", format)
	}

	return nil
}

func resourceApiManagementDiagnosticDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.DiagnosticClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	diagnosticId, err := parse.DiagnosticID(d.Id())
	if err != nil {
		return err
	}

	if resp, err := client.Delete(ctx, diagnosticId.ResourceGroup, diagnosticId.ServiceName, diagnosticId.Name, ""); err != nil {
		if !utils.ResponseWasNotFound(resp) {
			return fmt.Errorf("deleting Diagnostic %q (Resource Group %q / API Management Service %q): %+v", diagnosticId.Name, diagnosticId.ResourceGroup, diagnosticId.ServiceName, err)
		}
	}

	return nil
}
