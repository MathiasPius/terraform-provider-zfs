resource "zfs_pool" "zdata" {
  name = "zdata"

  mirror {
    device {
      path = "/dev/sda"
    }

    device {
      path = "/dev/sdb"
    }
  }

  mirror {
    device {
      path = "/dev/sdc"
    }

    device {
      path = "/dev/sdd"
    }
  }

  property {
    name  = "relatime"
    value = "on"
  }

  property {
    name  = "recordsize"
    value = "4K"
  }

  property {
    name  = "compression"
    value = "on"
  }
}