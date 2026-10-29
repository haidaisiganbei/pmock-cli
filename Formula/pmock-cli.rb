class PmockCli < Formula
  desc "PMOCK open API command line client"
  homepage "https://github.com/haidaisiganbei/pmock-cli"
  version "0.1.1"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_arm64.tar.gz"
      sha256 "da3e78aa0b50defa7a49065c3d6b36e9f7cabf995663d3476fb9c1e258b8b4d1"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_amd64.tar.gz"
      sha256 "6fa4d90d592c8c74f64ce99b5dba305bfe93f2273179f8b82212562e8f05df5b"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_arm64.tar.gz"
      sha256 "c30aa51508324efe4817a58497d1547bb2e9cca7b5c1d577bc2b26488c24c579"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_amd64.tar.gz"
      sha256 "060924a3db1453eab69eb58f8697b2969480610e2d4f725c19e9e2ccefd38671"
    end
  end

  def install
    bin.install "pmock-cli"
  end

  test do
    assert_match "0.1.1", shell_output("#{bin}/pmock-cli --version")
  end
end
