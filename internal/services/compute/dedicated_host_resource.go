package compute

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2020-12-01/compute"
	"github.com/hashicorp/go-azure-helpers/response"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/compute/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/compute/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceDedicatedHost() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceDedicatedHostCreate,
		Read:   resourceDedicatedHostRead,
		Update: resourceDedicatedHostUpdate,
		Delete: resourceDedicatedHostDelete,

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.DedicatedHostID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.DedicatedHostName(),
			},

			"location": azure.SchemaLocation(),

			"dedicated_host_group_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.DedicatedHostGroupID,
			},

			"sku_name": {
				Type:     pluginsdk.TypeString,
				ForceNew: true,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					"DSv3-Type1",
					"DSv3-Type2",
					"DSv4-Type1",
					"ESv3-Type1",
					"ESv3-Type2",
					"FSv2-Type2",
					"DASv4-Type1",
					"DCSv2-Type1",
					"DDSv4-Type1",
					"DSv3-Type1",
					"DSv3-Type2",
					"DSv3-Type3",
					"DSv4-Type1",
					"EASv4-Type1",
					"EDSv4-Type1",
					"ESv3-Type1",
					"ESv3-Type2",
					"ESv3-Type3",
					"ESv4-Type1",
					"FSv2-Type2",
					"FSv2-Type3",
					"LSv2-Type1",
					"MS-Type1",
					"MSm-Type1",
					"MSmv2-Type1",
					"MSv2-Type1",
					"NVASv4-Type1",
					"NVSv3-Type1",
				}, false),
			},

			"platform_fault_domain": {
				Type:     pluginsdk.TypeInt,
				ForceNew: true,
				Required: true,
			},

			"auto_replace_on_failure": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				Default:  true,
			},

			"license_type": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(compute.DedicatedHostLicenseTypesNone),
					string(compute.DedicatedHostLicenseTypesWindowsServerHybrid),
					string(compute.DedicatedHostLicenseTypesWindowsServerPerpetual),
				}, false),
				Default: string(compute.DedicatedHostLicenseTypesNone),
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceDedicatedHostCreate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Compute.DedicatedHostsClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	dedicatedHostGroupId, err := parse.DedicatedHostGroupID(d.Get("dedicated_host_group_id").(string))
	if err != nil {
		return err
	}

	resourceGroupName := dedicatedHostGroupId.ResourceGroup
	hostGroupName := dedicatedHostGroupId.HostGroupName

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroupName, hostGroupName, name, "")
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("Error checking for present of existing Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", name, hostGroupName, resourceGroupName, err)
			}
		}
		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_dedicated_host", *existing.ID)
		}
	}

	parameters := compute.DedicatedHost{
		Location: utils.String(azure.NormalizeLocation(d.Get("location").(string))),
		DedicatedHostProperties: &compute.DedicatedHostProperties{
			AutoReplaceOnFailure: utils.Bool(d.Get("auto_replace_on_failure").(bool)),
			LicenseType:          compute.DedicatedHostLicenseTypes(d.Get("license_type").(string)),
			PlatformFaultDomain:  utils.Int32(int32(d.Get("platform_fault_domain").(int))),
		},
		Sku: &compute.Sku{
			Name: utils.String(d.Get("sku_name").(string)),
		},
		Tags: tags.Expand(d.Get("tags").(map[string]interface{})),
	}

	future, err := client.CreateOrUpdate(ctx, resourceGroupName, hostGroupName, name, parameters)
	if err != nil {
		return fmt.Errorf("Error creating Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", name, hostGroupName, resourceGroupName, err)
	}
	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for creation of Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", name, hostGroupName, resourceGroupName, err)
	}

	resp, err := client.Get(ctx, resourceGroupName, hostGroupName, name, "")
	if err != nil {
		return fmt.Errorf("Error retrieving Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", name, hostGroupName, resourceGroupName, err)
	}
	if resp.ID == nil {
		return fmt.Errorf("Cannot read ID for Dedicated Host %q (Host Group Name %q / Resource Group %q)", name, hostGroupName, resourceGroupName)
	}
	d.SetId(*resp.ID)

	return resourceDedicatedHostRead(d, meta)
}

func resourceDedicatedHostRead(d *pluginsdk.ResourceData, meta interface{}) error {
	groupsClient := meta.(*clients.Client).Compute.DedicatedHostGroupsClient
	hostsClient := meta.(*clients.Client).Compute.DedicatedHostsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.DedicatedHostID(d.Id())
	if err != nil {
		return err
	}

	group, err := groupsClient.Get(ctx, id.ResourceGroup, id.HostGroupName, "")
	if err != nil {
		if utils.ResponseWasNotFound(group.Response) {
			log.Printf("[INFO] Parent Dedicated Host Group %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Dedicated Host Group %q (Resource Group %q): %+v", id.HostGroupName, id.ResourceGroup, err)
	}

	resp, err := hostsClient.Get(ctx, id.ResourceGroup, id.HostGroupName, id.HostName, "")
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Dedicated Host %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
	}

	d.Set("name", resp.Name)
	d.Set("dedicated_host_group_id", group.ID)

	if location := resp.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}
	d.Set("sku_name", resp.Sku.Name)
	if props := resp.DedicatedHostProperties; props != nil {
		d.Set("auto_replace_on_failure", props.AutoReplaceOnFailure)
		d.Set("license_type", props.LicenseType)

		platformFaultDomain := 0
		if props.PlatformFaultDomain != nil {
			platformFaultDomain = int(*props.PlatformFaultDomain)
		}
		d.Set("platform_fault_domain", platformFaultDomain)
	}

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceDedicatedHostUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Compute.DedicatedHostsClient
	ctx, cancel := timeouts.ForUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.DedicatedHostID(d.Id())
	if err != nil {
		return err
	}

	parameters := compute.DedicatedHostUpdate{
		DedicatedHostProperties: &compute.DedicatedHostProperties{
			AutoReplaceOnFailure: utils.Bool(d.Get("auto_replace_on_failure").(bool)),
			LicenseType:          compute.DedicatedHostLicenseTypes(d.Get("license_type").(string)),
		},
		Tags: tags.Expand(d.Get("tags").(map[string]interface{})),
	}

	future, err := client.Update(ctx, id.ResourceGroup, id.HostGroupName, id.HostName, parameters)
	if err != nil {
		return fmt.Errorf("Error updating Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
	}
	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for update of Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
	}

	return resourceDedicatedHostRead(d, meta)
}

func resourceDedicatedHostDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Compute.DedicatedHostsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.DedicatedHostID(d.Id())
	if err != nil {
		return err
	}

	future, err := client.Delete(ctx, id.ResourceGroup, id.HostGroupName, id.HostName)
	if err != nil {
		return fmt.Errorf("Error deleting Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		if !response.WasNotFound(future.Response()) {
			return fmt.Errorf("Error waiting for deleting Dedicated Host %q (Host Group Name %q / Resource Group %q): %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
		}
	}

	// API has bug, which appears to be eventually consistent. Tracked by this issue: https://github.com/Azure/azure-rest-api-specs/issues/8137
	log.Printf("[DEBUG] Waiting for Dedicated Host %q (Host Group Name %q / Resource Group %q) to disappear", id.HostName, id.HostGroupName, id.ResourceGroup)
	stateConf := &pluginsdk.StateChangeConf{
		Pending:                   []string{"Exists"},
		Target:                    []string{"NotFound"},
		Refresh:                   dedicatedHostDeletedRefreshFunc(ctx, client, id),
		MinTimeout:                10 * time.Second,
		ContinuousTargetOccurence: 20,
		Timeout:                   d.Timeout(pluginsdk.TimeoutDelete),
	}

	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		return fmt.Errorf("Error waiting for Dedicated Host %q (Host Group Name %q / Resource Group %q) to become available: %+v", id.HostName, id.HostGroupName, id.ResourceGroup, err)
	}

	return nil
}

func dedicatedHostDeletedRefreshFunc(ctx context.Context, client *compute.DedicatedHostsClient, id *parse.DedicatedHostId) pluginsdk.StateRefreshFunc {
	return func() (interface{}, string, error) {
		res, err := client.Get(ctx, id.ResourceGroup, id.HostGroupName, id.HostName, "")
		if err != nil {
			if utils.ResponseWasNotFound(res.Response) {
				return "NotFound", "NotFound", nil
			}

			return nil, "", fmt.Errorf("Error polling to check if the Dedicated Host has been deleted: %+v", err)
		}

		return res, "Exists", nil
	}
}
