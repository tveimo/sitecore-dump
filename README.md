# sitecore-dump
Utility to dump parts of, or the entire content of a Sitecore instance to local disk.

This code was part of a larger utility to migrate date from a sitecore 8.* instance into a kontent.ai headless CMS 
project. The purpose of the utility is to create an offline dump of item data which can then be further processed 
for migration, to avoid loading an online sitecore instance repeatedly, or if the sitecore instance is to be taken 
offline.

It uses the item rest api from sitecore to extract data into a flat directory structure with filenames constructed 
from the uuid of sitecore items. 

More information about this api can be found online, eg at https://doc.sitecore.com/xp/en/SdnArchive/upload/sdn5/modules/sitecore%20item%20web%20api/sitecore_item_web_api_developer_guide_sc65-66-usletter.pdf

run as eg

``
go run ./src/*.go --host cms.host.com -user arnie -pass beback -v
``

It will by default not write to disk, add the -w and -wb flags to write metadata and / or binary data.

Default directory names are output and binaries, these must exist prior.

The metadata files will take names such as

``
A705D262-5714-4880-9962-051E25F1416D-children.json
A705D262-5714-4880-9962-051E25F1416D.json
``

and

``A232BCEA-76E0-48A1-A738-A29D91BE7AA5.jpg``

for binaries.
