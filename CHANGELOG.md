
## 0.6.2
* @jaynis fixed zfs filesystem/dataset `data` sources attempting to retrieve "required" properties, when no such properties are listed for datasources.

## 0.6.1
* @kemoycampbell fixed pool imports.

## 0.6.0

### Breaking
* ⚠️ @techhazard replace `dataset` resource and datasource types with the more accurate `volume` and `filesystem` equivalents.
    Previous implementation assumed all datasets were filesystems, which meant volumes (`zvol`) could not be represented.

    **Migration**: I recommend using `state rm` and `import` commands to convert a `zfs_dataset` into a `zfs_filesystem`.
    
