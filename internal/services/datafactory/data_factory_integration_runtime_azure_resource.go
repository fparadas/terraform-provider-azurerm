package datafactory

import (
	"fmt"
	"regexp"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/datafactory/mgmt/2018-06-01/datafactory"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/datafactory/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceDataFactoryIntegrationRuntimeAzure() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceDataFactoryIntegrationRuntimeAzureCreateUpdate,
		Read:   resourceDataFactoryIntegrationRuntimeAzureRead,
		Update: resourceDataFactoryIntegrationRuntimeAzureCreateUpdate,
		Delete: resourceDataFactoryIntegrationRuntimeAzureDelete,
		// TODO: replace this with an importer which validates the ID during import
		Importer: pluginsdk.DefaultImporter(),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringMatch(
					regexp.MustCompile(`^([a-zA-Z0-9](-|-?[a-zA-Z0-9]+)+[a-zA-Z0-9])$`),
					`Invalid name for Azure Integration Runtime: minimum 3 characters, must start and end with a number or a letter, may only consist of letters, numbers and dashes and no consecutive dashes.`,
				),
			},

			"description": {
				Type:     pluginsdk.TypeString,
				Optional: true,
			},

			"data_factory_name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryName(),
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"location": azure.SchemaLocation(),

			"compute_type": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				Default:  string(datafactory.DataFlowComputeTypeGeneral),
				ValidateFunc: validation.StringInSlice([]string{
					string(datafactory.DataFlowComputeTypeGeneral),
					string(datafactory.DataFlowComputeTypeComputeOptimized),
					string(datafactory.DataFlowComputeTypeMemoryOptimized),
				}, false),
			},

			"core_count": {
				Type:     pluginsdk.TypeInt,
				Optional: true,
				Default:  8,
				ValidateFunc: validation.IntInSlice([]int{
					8, 16, 32, 48, 80, 144, 272,
				}),
			},

			"time_to_live_min": {
				Type:     pluginsdk.TypeInt,
				Optional: true,
				Default:  0,
			},

			"virtual_network_enabled": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceDataFactoryIntegrationRuntimeAzureCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.IntegrationRuntimesClient
	managedVirtualNetworksClient := meta.(*clients.Client).DataFactory.ManagedVirtualNetworksClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	factoryName := d.Get("data_factory_name").(string)
	resourceGroup := d.Get("resource_group_name").(string)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, factoryName, name, "")

		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for presence of existing Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q): %s", name, resourceGroup, factoryName, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_data_factory_integration_runtime_azure", *existing.ID)
		}
	}

	description := d.Get("description").(string)

	managedIntegrationRuntime := datafactory.ManagedIntegrationRuntime{
		Description: &description,
		Type:        datafactory.TypeBasicIntegrationRuntimeTypeManaged,
		ManagedIntegrationRuntimeTypeProperties: &datafactory.ManagedIntegrationRuntimeTypeProperties{
			ComputeProperties: expandDataFactoryIntegrationRuntimeAzureComputeProperties(d),
		},
	}

	if d.Get("virtual_network_enabled").(bool) {
		virtualNetworkName, err := getManagedVirtualNetworkName(ctx, managedVirtualNetworksClient, resourceGroup, factoryName)
		if err != nil {
			return err
		}
		if virtualNetworkName == nil {
			return fmt.Errorf("virtual network feature for azure integration runtime is only available after managed virtual network for this data factory is enabled")
		}
		managedIntegrationRuntime.ManagedVirtualNetwork = &datafactory.ManagedVirtualNetworkReference{
			Type:          utils.String("ManagedVirtualNetworkReference"),
			ReferenceName: virtualNetworkName,
		}
	}

	basicIntegrationRuntime, _ := managedIntegrationRuntime.AsBasicIntegrationRuntime()

	integrationRuntime := datafactory.IntegrationRuntimeResource{
		Name:       &name,
		Properties: basicIntegrationRuntime,
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroup, factoryName, name, integrationRuntime, ""); err != nil {
		return fmt.Errorf("Error creating/updating Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q): %+v", name, resourceGroup, factoryName, err)
	}

	resp, err := client.Get(ctx, resourceGroup, factoryName, name, "")
	if err != nil {
		return fmt.Errorf("Error retrieving Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q): %+v", name, resourceGroup, factoryName, err)
	}

	if resp.ID == nil {
		return fmt.Errorf("Cannot read Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q) ID", name, resourceGroup, factoryName)
	}

	d.SetId(*resp.ID)

	return resourceDataFactoryIntegrationRuntimeAzureRead(d, meta)
}

func resourceDataFactoryIntegrationRuntimeAzureRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.IntegrationRuntimesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	factoryName := id.Path["factories"]
	name := id.Path["integrationruntimes"]

	resp, err := client.Get(ctx, resourceGroup, factoryName, name, "")
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q): %+v", name, resourceGroup, factoryName, err)
	}

	d.Set("name", name)
	d.Set("data_factory_name", factoryName)
	d.Set("resource_group_name", resourceGroup)

	managedIntegrationRuntime, convertSuccess := resp.Properties.AsManagedIntegrationRuntime()
	if !convertSuccess {
		return fmt.Errorf("Error converting managed integration runtime to Azure integration runtime %q (Resource Group %q, Data Factory %q)", name, resourceGroup, factoryName)
	}

	if managedIntegrationRuntime.Description != nil {
		d.Set("description", managedIntegrationRuntime.Description)
	}

	virtualNetworkEnabled := false
	if managedIntegrationRuntime.ManagedVirtualNetwork != nil && managedIntegrationRuntime.ManagedVirtualNetwork.ReferenceName != nil {
		virtualNetworkEnabled = true
	}
	d.Set("virtual_network_enabled", virtualNetworkEnabled)

	if computeProps := managedIntegrationRuntime.ComputeProperties; computeProps != nil {
		if location := computeProps.Location; location != nil {
			d.Set("location", location)
		}

		if dataFlowProps := computeProps.DataFlowProperties; dataFlowProps != nil {
			if computeType := &dataFlowProps.ComputeType; computeType != nil {
				d.Set("compute_type", string(*computeType))
			}

			if coreCount := dataFlowProps.CoreCount; coreCount != nil {
				d.Set("core_count", coreCount)
			}

			if timeToLive := dataFlowProps.TimeToLive; timeToLive != nil {
				d.Set("time_to_live_min", timeToLive)
			}
		}
	}

	return nil
}

func resourceDataFactoryIntegrationRuntimeAzureDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.IntegrationRuntimesClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := azure.ParseAzureResourceID(d.Id())

	if err != nil {
		return err
	}

	resourceGroup := id.ResourceGroup
	factoryName := id.Path["factories"]
	name := id.Path["integrationruntimes"]

	response, err := client.Delete(ctx, resourceGroup, factoryName, name)

	if err != nil {
		if !utils.ResponseWasNotFound(response) {
			return fmt.Errorf("Error deleting Data Factory Azure Integration Runtime %q (Resource Group %q, Data Factory %q): %+v", name, resourceGroup, factoryName, err)
		}
	}

	return nil
}

func expandDataFactoryIntegrationRuntimeAzureComputeProperties(d *pluginsdk.ResourceData) *datafactory.IntegrationRuntimeComputeProperties {
	location := azure.NormalizeLocation(d.Get("location").(string))
	coreCount := int32(d.Get("core_count").(int))
	timeToLiveMin := int32(d.Get("time_to_live_min").(int))

	return &datafactory.IntegrationRuntimeComputeProperties{
		Location: &location,
		DataFlowProperties: &datafactory.IntegrationRuntimeDataFlowProperties{
			ComputeType: datafactory.DataFlowComputeType(d.Get("compute_type").(string)),
			CoreCount:   &coreCount,
			TimeToLive:  &timeToLiveMin,
		},
	}
}
