class PmockCli < Formula
  desc "PMOCK open API command line client"
  homepage "https://github.com/haidaisiganbei/pmock-cli"
  version "0.1.4"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_arm64.tar.gz"
      sha256 "688c45636dda48f260d408ae20d1eba85fd3d00d2494eab1d9817fddc70a5561"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_darwin_amd64.tar.gz"
      sha256 "246c1d11a6f82af0da215cacdac9edf891364104adf7a02863d015967567f8ed"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_arm64.tar.gz"
      sha256 "581434d1bb4c9d8732def16aed396708e0fd6c1771e486425f280220ddd5336b"
    else
      url "https://github.com/haidaisiganbei/pmock-cli/releases/download/v#{version}/pmock-cli_#{version}_linux_amd64.tar.gz"
      sha256 "9f1a235dcbcf5975bd854ad1f632e9dda3d0236841872fd7c1f01bcac4690b06"
    end
  end

  def install
    bin.install "pmock-cli"
  end

  test do
    assert_match "0.1.4", shell_output("#{bin}/pmock-cli --version")
  end
end
