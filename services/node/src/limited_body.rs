use anyhow::{anyhow, Error};
use bytes::Bytes;
use http_body::Body;
use http_body::Frame;
use std::pin::Pin;
use std::task::{Context, Poll};

/// A `Body` wrapper that stops reading after `limit` bytes and returns an error.
///
/// This is used for both request and response bodies to prevent OOM on unexpected large payloads.
pub struct LimitedBody<B> {
    inner: B,
    limit: u64,
    seen: u64,
    kind: &'static str,
}

impl<B> LimitedBody<B> {
    pub fn new(inner: B, limit: u64, kind: &'static str) -> Self {
        Self {
            inner,
            limit,
            seen: 0,
            kind,
        }
    }
}

impl<B> Body for LimitedBody<B>
where
    B: Body<Data = Bytes> + Unpin,
    B::Error: Into<Error>,
{
    type Data = Bytes;
    type Error = Error;

    fn poll_frame(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<Option<Result<Frame<Self::Data>, Self::Error>>> {
        let poll = Pin::new(&mut self.inner).poll_frame(cx);
        let frame = match poll {
            Poll::Pending => return Poll::Pending,
            Poll::Ready(None) => return Poll::Ready(None),
            Poll::Ready(Some(Err(e))) => return Poll::Ready(Some(Err(e.into()))),
            Poll::Ready(Some(Ok(f))) => f,
        };

        if let Some(data) = frame.data_ref() {
            let n = data.len() as u64;
            if self.limit > 0 && self.seen.saturating_add(n) > self.limit {
                return Poll::Ready(Some(Err(anyhow!(
                    "{} body too large (limit={} bytes)",
                    self.kind,
                    self.limit
                ))));
            }
            self.seen = self.seen.saturating_add(n);
        }

        Poll::Ready(Some(Ok(frame)))
    }
}

