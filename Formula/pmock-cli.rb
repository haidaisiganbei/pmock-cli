class PmockCli < Formula
  desc "PMOCK open API command line client"
  homepage "https://github.com/haidaisiganbei/pmock-cli"
  version "0.1.5"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_arm64.tar.gz"
      sha256 "22cb7c3aedbfce4c940a62710434448fe56d6930507b47904e887fd2e52a692e"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_amd64.tar.gz"
      sha256 "071c2ba20596bdd2886a56acd1a939b0f21da575ad26cf1218e90735246d1df5"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_arm64.tar.gz"
      sha256 "e4886649e736a15636851f69686aa329d8def1d064e301a01b0ecb14ec5eb33b"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_amd64.tar.gz"
      sha256 "d0c0a1a473b0204295b40c18fe8936fb54e9803bea596cba754799803884fa9e"
    end
  end

  def install
    bin.install "pmock-cli"
  end

  test do
    assert_match "0.1.5", shell_output("#{bin}/pmock-cli --version")
  end
end
