class PmockCli < Formula
  desc "PMOCK open API command line client"
  homepage "https://github.com/haidaisiganbei/pmock-cli"
  version "0.1.3"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_arm64.tar.gz"
      sha256 "dd2997838f2cf658c159167d36a90178bff6b95081b43f5077c4bac16856e249"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_amd64.tar.gz"
      sha256 "9442527778fe954bcc54176c4d907c0770441277f29512952b643b7831cf0b48"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_arm64.tar.gz"
      sha256 "b61f61b27a8273d95680c7cea0b28ffa38b8f6c40e4972f74d3d92f620e80005"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_amd64.tar.gz"
      sha256 "386aa507a7085f06ebe276a0e1dab5f7576b4a265ab2e7ff86dbf0db90a5038a"
    end
  end

  def install
    bin.install "pmock-cli"
  end

  test do
    assert_match "0.1.3", shell_output("#{bin}/pmock-cli --version")
  end
end
