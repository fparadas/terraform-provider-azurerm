package network

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/locks"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/network/migration"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceNetworkInterfaceApplicationSecurityGroupAssociation() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceNetworkInterfaceApplicationSecurityGroupAssociationCreate,
		Read:   resourceNetworkInterfaceApplicationSecurityGroupAssociationRead,
		Delete: resourceNetworkInterfaceApplicationSecurityGroupAssociationDelete,
		// TODO: replace this with an importer which validates the ID during import
		Importer: pluginsdk.DefaultImporter(),

		SchemaVersion: 1,
		StateUpgraders: pluginsdk.StateUpgrades(map[int]pluginsdk.StateUpgrade{
			0: migration.NetworkInterfaceApplicationSecurityGroupAssociationV0ToV1{},
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"network_interface_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"application_security_group_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: azure.ValidateResourceID,
			},
		},
	}
}

func resourceNetworkInterfaceApplicationSecurityGroupAssociationCreate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	log.Printf("[INFO] preparing arguments for Network Interface <-> Application Security Group Association creation.")

	networkInterfaceId := d.Get("network_interface_id").(string)
	applicationSecurityGroupId := d.Get("application_security_group_id").(string)

	id, err := azure.ParseAzureResourceID(networkInterfaceId)
	if err != nil {
		return err
	}

	networkInterfaceName := id.Path["networkInterfaces"]
	resourceGroup := id.ResourceGroup

	locks.ByName(networkInterfaceName, networkInterfaceResourceName)
	defer locks.UnlockByName(networkInterfaceName, networkInterfaceResourceName)

	read, err := client.Get(ctx, resourceGroup, networkInterfaceName, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			log.Printf("[INFO] Network Interface %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	props := read.InterfacePropertiesFormat
	if props == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}
	if props.IPConfigurations == nil {
		return fmt.Errorf("Error: `properties.ipConfigurations` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}

	info := parseFieldsFromNetworkInterface(*props)
	resourceId := fmt.Sprintf("%s|%s", networkInterfaceId, applicationSecurityGroupId)
	if utils.SliceContainsValue(info.applicationSecurityGroupIDs, applicationSecurityGroupId) {
		return tf.ImportAsExistsError("azurerm_network_interface_application_security_group_association", resourceId)
	}

	info.applicationSecurityGroupIDs = append(info.applicationSecurityGroupIDs, applicationSecurityGroupId)

	read.InterfacePropertiesFormat.IPConfigurations = mapFieldsToNetworkInterface(props.IPConfigurations, info)

	future, err := client.CreateOrUpdate(ctx, resourceGroup, networkInterfaceName, read)
	if err != nil {
		return fmt.Errorf("Error updating Application Security Group Association for Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for completion of Application Security Group Association for NIC %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	d.SetId(resourceId)

	return resourceNetworkInterfaceApplicationSecurityGroupAssociationRead(d, meta)
}

func resourceNetworkInterfaceApplicationSecurityGroupAssociationRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	splitId := strings.Split(d.Id(), "|")
	if len(splitId) != 2 {
		return fmt.Errorf("Expected ID to be in the format {networkInterfaceId}|{applicationSecurityGroupId} but got %q", d.Id())
	}

	nicID, err := azure.ParseAzureResourceID(splitId[0])
	if err != nil {
		return err
	}

	networkInterfaceName := nicID.Path["networkInterfaces"]
	resourceGroup := nicID.ResourceGroup
	applicationSecurityGroupId := splitId[1]

	read, err := client.Get(ctx, resourceGroup, networkInterfaceName, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			log.Printf("[DEBUG] Network Interface %q (Resource Group %q) was not found - removing from state!", networkInterfaceName, resourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	nicProps := read.InterfacePropertiesFormat
	if nicProps == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}

	info := parseFieldsFromNetworkInterface(*nicProps)
	exists := false
	for _, groupId := range info.applicationSecurityGroupIDs {
		if groupId == applicationSecurityGroupId {
			exists = true
		}
	}

	if !exists {
		log.Printf("[DEBUG] Association between Network Interface %q (Resource Group %q) and Application Security Group %q was not found - removing from state!", networkInterfaceName, resourceGroup, applicationSecurityGroupId)
		d.SetId("")
		return nil
	}

	d.Set("application_security_group_id", applicationSecurityGroupId)
	d.Set("network_interface_id", read.ID)

	return nil
}

func resourceNetworkInterfaceApplicationSecurityGroupAssociationDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.InterfacesClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	splitId := strings.Split(d.Id(), "|")
	if len(splitId) != 2 {
		return fmt.Errorf("Expected ID to be in the format {networkInterfaceId}|{applicationSecurityGroupId} but got %q", d.Id())
	}

	nicID, err := azure.ParseAzureResourceID(splitId[0])
	if err != nil {
		return err
	}

	networkInterfaceName := nicID.Path["networkInterfaces"]
	resourceGroup := nicID.ResourceGroup
	applicationSecurityGroupId := splitId[1]

	locks.ByName(networkInterfaceName, networkInterfaceResourceName)
	defer locks.UnlockByName(networkInterfaceName, networkInterfaceResourceName)

	read, err := client.Get(ctx, resourceGroup, networkInterfaceName, "")
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			return fmt.Errorf("Network Interface %q (Resource Group %q) was not found!", networkInterfaceName, resourceGroup)
		}

		return fmt.Errorf("Error retrieving Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	props := read.InterfacePropertiesFormat
	if props == nil {
		return fmt.Errorf("Error: `properties` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}

	if props.IPConfigurations == nil {
		return fmt.Errorf("Error: `properties.ipConfigurations` was nil for Network Interface %q (Resource Group %q)", networkInterfaceName, resourceGroup)
	}

	info := parseFieldsFromNetworkInterface(*props)

	applicationSecurityGroupIds := make([]string, 0)
	for _, v := range info.applicationSecurityGroupIDs {
		if v != applicationSecurityGroupId {
			applicationSecurityGroupIds = append(applicationSecurityGroupIds, v)
		}
	}
	info.applicationSecurityGroupIDs = applicationSecurityGroupIds
	read.InterfacePropertiesFormat.IPConfigurations = mapFieldsToNetworkInterface(props.IPConfigurations, info)

	future, err := client.CreateOrUpdate(ctx, resourceGroup, networkInterfaceName, read)
	if err != nil {
		return fmt.Errorf("Error removing Application Security Group for Network Interface %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("Error waiting for removal of Application Security Group for NIC %q (Resource Group %q): %+v", networkInterfaceName, resourceGroup, err)
	}

	return nil
}
