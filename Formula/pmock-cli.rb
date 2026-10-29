class PmockCli < Formula
  desc "PMOCK open API command line client"
  homepage "https://github.com/haidaisiganbei/pmock-cli"
  version "0.1.2"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_arm64.tar.gz"
      sha256 "1c0367323213a845b4df69b40cc5d0d3938cc5b488c66b402b9f49a8af7c6b41"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_amd64.tar.gz"
      sha256 "6eddb85ce98b2124bd32780c55bb9b0dec7fa7f30971256fe34ca2929188c4ec"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_arm64.tar.gz"
      sha256 "947006a1eb555cd09dea51ebe5276aba70111eda67f44c0649c5add76b18e6ee"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_amd64.tar.gz"
      sha256 "8996c3f983b9c27637b273bb151dde238772b882198889f7b4e7afeb60e76810"
    end
  end

  def install
    bin.install "pmock-cli"
  end

  test do
    assert_match "0.1.2", shell_output("#{bin}/pmock-cli --version")
  end
end
